package sources

import (
	"context"
	"fmt"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"net/url"
	"strconv"
	"time"
)

// Tasks reads completed tasks in Day's timezone, with complete pagination.
type Tasks struct{ Conn pocket.Conn }

func NewTasks(conn pocket.Conn) *Tasks { return &Tasks{Conn: conn} }
func (p *Tasks) Name() string          { return "tasks" }
func (p *Tasks) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := c.Health(ctx)
	return "Pocket Tasks " + v, err
}
func (p *Tasks) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	start, end, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	act := Activity{}
	for offset := 0; ; {
		var page struct {
			Tasks []struct {
				ID          int64   `json:"id"`
				ListID      int64   `json:"list_id"`
				Title       string  `json:"title"`
				CompletedAt *string `json:"completed_at"`
				List        *struct {
					Name string `json:"name"`
				} `json:"list"`
			} `json:"tasks"`
			Total int `json:"total"`
		}
		q := url.Values{"status": {"done"}, "modified_since": {start.UTC().Format(time.RFC3339)}, "limit": {"500"}, "offset": {strconv.Itoa(offset)}}
		if err := c.Get(ctx, "/api/v1/tasks", q, &page); err != nil {
			return nil, err
		}
		for _, task := range page.Tasks {
			if task.CompletedAt == nil {
				continue
			}
			ts, ok := ParseAny(*task.CompletedAt)
			if !ok || ts.Before(start) || !ts.Before(end) {
				continue
			}
			group := ""
			if task.List != nil {
				group = task.List.Name
			}
			act.Items = append(act.Items, ActivityItem{Title: task.Title, Group: group, Done: true, Time: LocalClock(ts, loc), Ref: fmt.Sprint(task.ID)})
		}
		offset += len(page.Tasks)
		if offset >= page.Total || len(page.Tasks) == 0 {
			break
		}
	}
	act.Finish()
	return &Result{Payload: act, Headline: float64(act.Done), Summary: Plural(act.Done, "task completed", "tasks completed")}, nil
}
