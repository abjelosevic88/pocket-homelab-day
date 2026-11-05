package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeOpenAI answers /audio/transcriptions and counts the calls.
type fakeOpenAI struct {
	*httptest.Server
	mu    sync.Mutex
	calls int
}

func newFakeOpenAI(t *testing.T) *fakeOpenAI {
	f := &fakeOpenAI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" || r.Header.Get("Authorization") != "Bearer sk-test" {
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		f.mu.Lock()
		f.calls++
		f.mu.Unlock()
		_, _ = rw.Write([]byte(`{"text":"From OpenAI"}`))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeOpenAI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// dictate posts a recording to /ai/transcribe as the browser does.
func (e *linkEnv) dictate(tok, audio string) (int, map[string]string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("language", "bs")
	fw, _ := mw.CreateFormFile("file", "dictation.webm")
	_, _ = fw.Write([]byte(audio))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.ts.URL+"/api/v1/ai/transcribe", &buf)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	out := map[string]string{}
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

// dictation is what Settings → AI says about dictation.
func (e *linkEnv) dictation(tok string) (dictationState, bool) {
	e.t.Helper()
	_, body := e.do(tok, "GET", "/settings", nil)
	var v struct {
		Dictation  dictationState `json:"dictation"`
		Configured bool           `json:"ai_transcribe_configured"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		e.t.Fatalf("settings: %v %s", err, body)
	}
	return v.Dictation, v.Configured
}

func (e *linkEnv) asked(token string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, tk := range e.tokens {
		if tk == token {
			n++
		}
	}
	return n
}

// With the Day → Pocket Voice voice:transcribe link on, dictation is written
// out by Pocket Voice (a fresh link token, the recording to the box), even with
// an OpenAI key; with the link off it goes to OpenAI as before, and without
// either there is no dictation.
func TestDictationByPocketVoice(t *testing.T) {
	e := newLinkEnv(t, true)
	oa := newFakeOpenAI(t)

	d, on := e.dictation(e.ownerTok)
	if d.Provider != "voice" || d.Via != "core" || d.Detail != "" || !on {
		t.Fatalf("link on: %+v %v", d, on)
	}
	code, out := e.dictate(e.ownerTok, "dentist at nine")
	if code != 200 || out["text"] != "Heard: dentist at nine (bs)" || out["by"] != "voice" {
		t.Fatalf("dictation through Pocket Voice: %d %v", code, out)
	}
	if last := e.lastCall("voice"); last != "Bearer pcx_voice_voice:transcribe /api/v1/transcribe" {
		t.Errorf("Pocket Voice called with %q", last)
	}
	if n := e.asked("owner@x voice voice:transcribe"); n != 1 {
		t.Errorf("voice:transcribe token asked %d times", n)
	}
	// Pocket Voice could not write it out: its words, no OpenAI
	code, out = e.dictate(e.ownerTok, "silence")
	if code != http.StatusBadGateway || out["error"] != "Pocket Voice could not write it out: nothing was said in the recording" {
		t.Errorf("422 from Pocket Voice: %d %v", code, out)
	}

	// The link switched off in Core (as new links arrive): no dictation without a key …
	e.mu.Lock()
	e.off["voice voice:transcribe"] = "off_admin"
	e.mu.Unlock()
	d, on = e.dictation(e.ownerTok)
	if d.Provider != "" || on || !strings.Contains(d.Detail, "“Write out speech on this server” link is switched off in Pocket Core (Links)") {
		t.Fatalf("link off, no key: %+v %v", d, on)
	}
	calls := e.callCount("voice")
	if code, out = e.dictate(e.ownerTok, "x"); code != http.StatusBadRequest || !strings.Contains(out["error"], "Dictation needs Pocket Voice") {
		t.Errorf("no dictation: %d %v", code, out)
	}
	// … and OpenAI with one.
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"ai_key": "sk-test", "ai_base_url": oa.URL})
	d, on = e.dictation(e.ownerTok)
	if d.Provider != "openai" || !on || d.Detail == "" {
		t.Fatalf("link off, OpenAI key: %+v %v", d, on)
	}
	if code, out = e.dictate(e.ownerTok, "x"); code != 200 || out["text"] != "From OpenAI" || oa.count() != 1 {
		t.Errorf("OpenAI dictation: %d %v (%d calls)", code, out, oa.count())
	}
	if e.callCount("voice") != calls {
		t.Error("Pocket Voice called with its link off")
	}

	// Link on again: Pocket Voice comes first, the key stays unused.
	e.mu.Lock()
	delete(e.off, "voice voice:transcribe")
	e.mu.Unlock()
	if d, _ = e.dictation(e.ownerTok); d.Provider != "voice" {
		t.Fatalf("link back on: %+v", d)
	}
	if code, out = e.dictate(e.ownerTok, "hello"); code != 200 || out["by"] != "voice" || oa.count() != 1 {
		t.Errorf("Pocket Voice before OpenAI: %d %v (%d OpenAI calls)", code, out, oa.count())
	}

	// Pocket Core not answering: the recording is not sent anywhere, OpenAI neither.
	e.mu.Lock()
	e.tokDown = true
	e.mu.Unlock()
	if code, out = e.dictate(e.ownerTok, "hello"); code != http.StatusBadGateway || !strings.Contains(out["error"], "Pocket Core did not answer, so the recording was not sent anywhere") || oa.count() != 1 {
		t.Errorf("Core down: %d %v (%d OpenAI calls)", code, out, oa.count())
	}
	e.mu.Lock()
	e.tokDown = false
	e.mu.Unlock()

	// Direct: the Pocket Voice source's own URL and token, nothing asked of Core.
	asked := e.asked("owner@x voice voice:transcribe")
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"voice": map[string]any{"mode": "direct", "url": e.targets["voice"].URL, "key": "direct-voice"}}})
	if d, _ = e.dictation(e.ownerTok); d.Provider != "voice" || d.Via != "direct" {
		t.Fatalf("direct: %+v", d)
	}
	if code, out = e.dictate(e.ownerTok, "direct"); code != 200 || out["text"] != "Heard: direct (bs)" {
		t.Errorf("direct dictation: %d %v", code, out)
	}
	if last := e.lastCall("voice"); last != "Bearer direct-voice /api/v1/transcribe" {
		t.Errorf("direct call %q", last)
	}
	if e.asked("owner@x voice voice:transcribe") != asked {
		t.Error("asked Core for a token on Direct")
	}
	// Direct without the token: not Pocket Voice, and the person reads why.
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"voice": map[string]any{"key": ""}}})
	if d, _ = e.dictation(e.ownerTok); d.Provider != "openai" || d.Detail != "the Pocket Voice source is on Direct and has no token yet" {
		t.Errorf("direct without a token: %+v", d)
	}

	// The member is not in Pocket Voice's audience in Core.
	if d, on = e.dictation(e.memberTok); d.Provider != "" || on || !strings.Contains(d.Detail, "do not have access to Pocket Voice") {
		t.Errorf("member: %+v %v", d, on)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.bad) > 0 {
		t.Errorf("calls outside the scopes: %v", e.bad)
	}
}

// Without Pocket Core (CORE_URL empty) dictation is OpenAI's, or Pocket
// Voice's with its URL and token in Settings → Sources.
func TestDictationWithoutCore(t *testing.T) {
	e := newLinkEnv(t, false)
	oa := newFakeOpenAI(t)
	if d, on := e.dictation(e.ownerTok); d.Provider != "" || on || d.Detail != "the Pocket Voice source is on Direct and has no token yet" {
		t.Fatalf("nothing set: %+v %v", d, on)
	}
	if code, _ := e.dictate(e.ownerTok, "x"); code != http.StatusBadRequest {
		t.Errorf("no dictation: %d", code)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"ai_key": "sk-test", "ai_base_url": oa.URL})
	if d, on := e.dictation(e.ownerTok); d.Provider != "openai" || !on {
		t.Fatalf("OpenAI key: %+v %v", d, on)
	}
	if code, out := e.dictate(e.ownerTok, "x"); code != 200 || out["text"] != "From OpenAI" {
		t.Errorf("OpenAI dictation: %d %v", code, out)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"voice": map[string]any{"url": e.targets["voice"].URL, "key": "direct-voice"}}})
	if d, _ := e.dictation(e.ownerTok); d.Provider != "voice" || d.Via != "direct" {
		t.Fatalf("Pocket Voice direct: %+v", d)
	}
	if code, out := e.dictate(e.ownerTok, "local"); code != 200 || out["text"] != "Heard: local (bs)" || oa.count() != 1 {
		t.Errorf("Pocket Voice dictation: %d %v (%d OpenAI calls)", code, out, oa.count())
	}
}
