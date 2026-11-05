package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// ---- dictation: Pocket Voice on the box, else OpenAI ----
//
// Speaking an entry or a moment (POST /api/v1/ai/transcribe) is written out
// by Pocket Voice when Day may use it for the person, so the recording stays
// on the box and nothing is kept there:
//
//   - the Pocket Voice source on Direct, with its URL and token, or else
//   - the Day → Pocket Voice "voice:transcribe" link switched on in Pocket
//     Core (new links arrive switched off: until it is on in Core → Links,
//     dictation stays where it was).
//
// Otherwise it goes to the person's OpenAI-compatible endpoint, as before.
// While Pocket Core does not answer the recording is not sent anywhere: it is
// not quietly handed to OpenAI instead (the same as for Pocket AI).

// voiceTranscribeScope is what Day asks of Pocket Voice besides voice:read.
const voiceTranscribeScope = "voice:transcribe"

// voiceTimeout is how long Pocket Voice may take to write a recording out on
// the box (within the route's three minutes).
const voiceTimeout = 170 * time.Second

// dictationState is who writes out a person's dictation, for Settings → AI.
type dictationState struct {
	Provider string `json:"provider"`         // "voice" | "openai" | "" (no dictation)
	Via      string `json:"via,omitempty"`    // Pocket Voice: "core" | "direct"
	Detail   string `json:"detail,omitempty"` // why Pocket Voice does not ("" when it does)
	Mode     string `json:"mode,omitempty"`   // the Pocket Voice source's mode: Direct is never through Core
}

// dictationFor sums up a person's dictation from the Pocket Voice source's
// state and whether they have an OpenAI key.
func dictationFor(voice sourceState, openAI bool) dictationState {
	d := dictationState{Via: voice.DictationVia, Detail: voice.DictationDetail, Mode: voice.Mode}
	switch {
	case d.Via != "":
		d.Provider, d.Detail = "voice", ""
	case openAI:
		d.Provider = "openai"
	}
	return d
}

// applyDictation adds to the Pocket Voice source's state whether Pocket Voice
// writes out the person's dictation: on Direct with the source's URL and
// token, otherwise (Pocket Core or Off) through the voice:transcribe link.
func applyDictation(ss *sourceState, coreOK bool, links func() (map[string]core.LinkState, error)) {
	switch {
	case ss.Mode == modeDirect && ss.Via == modeDirect:
		ss.DictationVia = modeDirect
		return
	case ss.Mode == modeDirect:
		ss.DictationDetail = "the Pocket Voice source is on Direct and has " + firstNonEmpty(ss.Detail, "no working connection")
		return
	case !coreOK:
		ss.DictationDetail = "there is no Pocket Core on this box"
		return
	}
	all, err := links()
	l, ok := all[pocketSources["voice"].slug+" "+voiceTranscribeScope]
	switch {
	case err != nil:
		ss.DictationDetail = "Pocket Core did not answer: " + err.Error()
	case ok && l.On:
		ss.DictationVia = modeCore
	case !ok && len(all) == 0:
		ss.DictationDetail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
	case !ok:
		ss.DictationDetail = linkReason("Pocket Voice", "not_declared")
	default:
		ss.DictationDetail = dictationLinkReason(l.State, l.Detail)
	}
}

// dictationLinkReason says why the voice:transcribe link does not work, naming
// it apart from the journal's voice:read link.
func dictationLinkReason(reason, detail string) string {
	const link = "the Day → Pocket Voice “Write out speech on this server” link"
	switch reason {
	case "off_admin":
		return link + " is switched off in Pocket Core (Links)"
	case "off_person":
		return "you switched " + link + " off in Pocket Core (Settings → Apps acting for me)"
	}
	return firstNonEmpty(linkReason("Pocket Voice", reason), detail, reason)
}

// voiceDictation returns the client to the person's Pocket Voice when it
// writes out their dictation, nil when it does not; an error when Pocket Core
// could not say.
func (s *Server) voiceDictation(ctx context.Context, u *store.User, st store.Settings) (*pocket.Client, error) {
	src := st.Sources["voice"]
	coreOK := s.core.Configured()
	if connMode("voice", src, coreOK) == modeDirect {
		if s.directDetail(ctx, "voice", src) != "" {
			return nil, nil
		}
		base := src.URL
		if base == "" {
			base = s.resolveSibling(ctx, pocketSources["voice"].slug)
		}
		return pocket.New(base, src.Key, voiceTimeout), nil
	}
	if !coreOK || u == nil || u.Email == "" {
		return nil, nil
	}
	tok, err := s.core.ActAs(ctx, u.Email, pocketSources["voice"].slug, voiceTranscribeScope)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			return nil, nil // the link is off (or Pocket Voice is not there): OpenAI, as before
		}
		return nil, err
	}
	return pocket.New(tok.URL, tok.AccessToken, voiceTimeout), nil
}

// voiceTranscribe sends a recording to Pocket Voice and returns its words and
// the engine that wrote them out ("whisper base"). Pocket Voice keeps nothing.
func voiceTranscribe(ctx context.Context, cl *pocket.Client, audio []byte, filename, contentType, lang string) (string, string, error) {
	var out struct {
		Text     string `json:"text"`
		Language string `json:"language"`
		Engine   string `json:"engine"`
	}
	if err := cl.PostFile(ctx, "/api/v1/transcribe", filename, contentType, audio, map[string]string{"language": lang}, &out); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(out.Text), out.Engine, nil
}

// voiceError is what the person reads when Pocket Voice did not write it out.
func voiceError(err error) string {
	var he *pocket.HTTPError
	switch {
	case errors.As(err, &he) && he.Status == http.StatusUnprocessableEntity:
		return "Pocket Voice could not write it out: " + firstNonEmpty(he.Message, "it gave no reason")
	case errors.As(err, &he) && he.Message != "":
		return "Pocket Voice did not take the recording: " + he.Message
	case errors.Is(err, pocket.ErrNotFound):
		return "this Pocket Voice version cannot write out dictation for other apps yet"
	}
	return "Pocket Voice did not answer: " + err.Error()
}
