package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/live"
	"github.com/thorstenpfister/basho/internal/service"
)

// LiveMsg is a live poll's answer, sent to every screen on the stack
// (SPEC §10.2): today's bouts of both divisions, already merged into the
// service's cache, bout index and ratings.
type LiveMsg struct{ Poll service.LivePoll }

// Messages of live mode inside the root.
type (
	liveInfoMsg struct {
		id    domain.BashoID
		start domain.Date
		ok    bool
	}
	liveTickMsg     struct{ gen int } // the scheduler's timer
	liveSnapshotMsg struct {
		poll service.LivePoll
		err  error
	}
	liveClockMsg struct{ gen int } // redraws the countdown
)

// liveState is the root's part of live mode.
type liveState struct {
	sched     *live.Scheduler // nil until the current tournament is known
	id        domain.BashoID
	manualOff bool // L turned it off: live.auto leaves it off today
	polling   bool
	gen       int // drops timers of an earlier schedule
	last      *service.LivePoll
	refreshed time.Time // the last R
}

// liveRefreshCooldown is the cooldown of R (SPEC §8.8).
const liveRefreshCooldown = 30 * time.Second

// loadLive finds the tournament in progress, for the scheduler.
func (m Model) loadLive() tea.Cmd {
	svc := m.ctx.Svc
	if svc == nil {
		return nil
	}
	return func() tea.Msg {
		id, start, ok, err := svc.LiveTournament(service.CachedOnly(context.Background()))
		if err != nil {
			id, start, ok, err = svc.LiveTournament(context.Background())
		}
		if err != nil {
			ok = false
		}
		return liveInfoMsg{id, start, ok}
	}
}

func (m Model) liveConfig() live.Config {
	l := m.ctx.Cfg.Live
	cfg := live.Config{
		Interval: time.Duration(l.Interval),
		Window: domain.LiveWindow{
			Start: time.Duration(l.WindowStart), End: time.Duration(l.WindowEnd),
			FinalDayEnd: time.Duration(l.WindowEnd) + 30*time.Minute,
		},
		Rand: m.ctx.Rand,
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Float64
	}
	return cfg
}

// liveUpdate handles live mode's messages; ok is false for other messages.
func (m Model) liveUpdate(msg tea.Msg) (Model, tea.Cmd, bool) {
	now := m.ctx.Clock.Now()
	switch msg := msg.(type) {
	case liveInfoMsg:
		var start domain.Date
		if msg.ok {
			start = msg.start
		}
		m.live.sched, m.live.id = live.New(m.liveConfig(), start), msg.id
		if m.ctx.Cfg.Live.Auto && m.live.sched.InWindow(now) {
			m.live.sched.SetOn(true)
		}
		return m.liveTick(now)
	case liveTickMsg:
		if msg.gen != m.live.gen || m.live.sched == nil {
			return m, nil, true
		}
		return m.liveTick(now)
	case liveClockMsg:
		if msg.gen != m.live.gen || m.live.sched == nil || !m.live.sched.On() {
			return m, nil, true
		}
		return m, m.liveClock(), true
	case liveSnapshotMsg:
		m.live.polling = false
		if m.live.sched == nil {
			return m, nil, true
		}
		r := live.Result{Err: msg.err}
		var broadcast tea.Cmd
		if msg.err == nil {
			p := msg.poll
			m.live.last = &p
			r = live.Result{UpdatedAt: p.UpdatedAt, Complete: p.Complete, Err: p.Err()}
			if p.Prov.Failed != "" {
				r.Fallback = p.Prov.Source
			}
			next, cmd := m.broadcast(LiveMsg{p})
			m, broadcast = next, cmd
		}
		m.live.sched.Polled(now, r)
		next, cmd, _ := m.liveTick(now)
		return next, tea.Batch(broadcast, cmd), true
	}
	return m, nil, false
}

// liveTick turns live mode on when its window opens (live.auto), polls
// when due, and sets the timer for the next look.
func (m Model) liveTick(now time.Time) (Model, tea.Cmd, bool) {
	s := m.live.sched
	if !s.On() && m.ctx.Cfg.Live.Auto && !m.live.manualOff && s.InWindow(now) {
		s.SetOn(true)
	}
	var poll tea.Cmd
	if s.Due(now) && !m.live.polling {
		m.live.polling = true
		svc, id, day := m.ctx.Svc, m.live.id, s.DayNumber(now)
		poll = func() tea.Msg {
			p, err := svc.LivePoll(context.Background(), id, day)
			return liveSnapshotMsg{p, err}
		}
	}
	m.live.gen++
	wake := s.Wake(now)
	if !s.On() && m.ctx.Cfg.Live.Auto && !m.live.manualOff {
		wake = s.NextOpen(now)
	}
	var timer tea.Cmd
	if !wake.IsZero() && !m.live.polling {
		gen := m.live.gen
		timer = m.ctx.After(max(wake.Sub(now), 0), func(time.Time) tea.Msg { return liveTickMsg{gen} })
	}
	return m, tea.Batch(poll, timer, m.liveClock()), true
}

// liveClock redraws the status bar every second while live mode is on.
func (m Model) liveClock() tea.Cmd {
	if m.live.sched == nil || !m.live.sched.On() {
		return nil
	}
	gen := m.live.gen
	return m.ctx.After(time.Second, func(time.Time) tea.Msg { return liveClockMsg{gen} })
}

// toggleLive is L.
func (m Model) toggleLive() (Model, tea.Cmd) {
	s := m.live.sched
	if s == nil {
		return m, Message("Live mode: still looking for the current tournament")
	}
	on := !s.On()
	s.SetOn(on)
	m.live.manualOff = !on
	next, cmd, _ := m.liveTick(m.ctx.Clock.Now())
	return next, cmd
}

// refreshLive is R while live mode polls: the next poll runs now, at the
// base interval, once per 30 s.
func (m Model) refreshLive() (Model, tea.Cmd) {
	now := m.ctx.Clock.Now()
	if !m.live.refreshed.IsZero() && now.Sub(m.live.refreshed) < liveRefreshCooldown {
		wait := (liveRefreshCooldown - now.Sub(m.live.refreshed)).Round(time.Second)
		return m, Message(fmt.Sprintf("Refreshed a moment ago; again in %s", wait))
	}
	m.live.refreshed = now
	m.live.sched.Reset()
	next, cmd, _ := m.liveTick(now)
	return next, cmd
}

// livePolls reports whether live mode is polling now: R goes to it.
func (m Model) livePolls() bool {
	if m.live.sched == nil {
		return false
	}
	st := m.live.sched.State(m.ctx.Clock.Now())
	return st == live.Polling || st == live.Backoff
}

// broadcast sends a message to every screen on the stack, top last.
func (m Model) broadcast(msg tea.Msg) (Model, tea.Cmd) {
	stack := make([]Screen, len(m.stack))
	cmds := make([]tea.Cmd, len(m.stack))
	for i, s := range m.stack {
		stack[i], cmds[i] = s.Update(m.ctx, msg)
	}
	m.stack = stack
	return m, tea.Batch(cmds...)
}

// liveStatus is the status bar's live part (SPEC §10.3); empty when off.
func (m Model) liveStatus() string {
	if m.live.sched == nil {
		return ""
	}
	t := m.ctx.Theme
	now := m.ctx.Clock.Now()
	st := m.live.sched.Status(now)
	clock := func(at time.Time) string { return at.In(domain.JST).Format("15:04 JST") }
	countdown := func(at time.Time) string {
		d := max(at.Sub(now), 0).Round(time.Second)
		return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
	}
	switch st.State {
	case live.Waiting:
		return t.Dim.Render(t.LiveWait+" LIVE") + t.Dim.Render(fmt.Sprintf(" · window opens %s (in %s)", clock(st.Next), until(st.Next.Sub(now))))
	case live.Polling:
		return t.Red.Render(t.Live+" LIVE") + " · next " + countdown(st.Next) + m.liveSource(now)
	case live.Backoff:
		what := "the sources are unreachable"
		if p := m.live.last; st.Fallback != "" && p != nil && p.Prov.Failed != "" {
			what = fmt.Sprintf("%s unreachable, using %s", p.Prov.Failed, st.Fallback)
		}
		return t.Red.Render(t.Live+" LIVE") + " · " + t.Gold.Render(t.Stale+" "+what) + " · retry " + countdown(st.Next)
	case live.Done:
		if !st.FinalCheck.IsZero() {
			return t.Green.Render(t.Complete+" LIVE") + " · day complete · final check " + st.FinalCheck.In(domain.JST).Format("15:04") + m.liveSource(now)
		}
		if m.live.last != nil && m.live.last.Complete {
			return t.Green.Render(t.Complete+" LIVE") + " · day complete" + m.liveSource(now)
		}
		return t.Dim.Render(t.Complete + " LIVE · window closed")
	case live.Unavailable:
		today := domain.DateOf(now)
		next := domain.UpcomingTournament(today)
		if today.Compare(next.EstimatedStart()) >= 0 {
			next = next.Next()
		}
		return t.Dim.Render(fmt.Sprintf("LIVE unavailable · next basho %s, Day 1 %s", next.Name(), next.EstimatedStart()))
	}
	return ""
}

// liveSource names the last poll's source and its update time:
// " · osada 16:42 JST (1m ago)".
func (m Model) liveSource(now time.Time) string {
	p := m.live.last
	if p == nil {
		return ""
	}
	at := p.Prov.FetchedAt
	if p.Prov.SourceUpdatedAt != nil {
		at = *p.Prov.SourceUpdatedAt
	}
	return fmt.Sprintf(" · %s %s (%s)", p.Prov.Source, at.In(domain.JST).Format("15:04 JST"), age(now.Sub(at)))
}

// until is a short duration: "2h13m", "45m".
func until(d time.Duration) string {
	d = d.Round(time.Minute)
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
