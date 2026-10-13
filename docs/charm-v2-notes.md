# Charm v2 notes

The Charm v2 API facts basho relies on. They were checked against the module sources on 2026-10-13 and confirmed by building a throwaway model driven by teatest. Versions live in `go.mod`, and SPEC §5.9 lists which modules are used. Re-check this file when bumping a Charm module. Much of v2 differs from v1, so code from memory or v1 tutorials is likely wrong.

## Import paths

- Bubble Tea, Lip Gloss and Bubbles: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`.
- The `x` modules (`teatest`, `golden`, later `mosaic`): use the **`github.com/charmbracelet/x/...`** path. Their `go.mod` declares it, so the `charm.land/x/...` aliases fail with a module path mismatch.
- Use `github.com/charmbracelet/x/exp/teatest/v2`. The module without `/v2` is built on Bubble Tea v1.
- Fang v1.0.0 requires a Lip Gloss v2 beta; module selection raises it to the version in `go.mod`.

## Bubble Tea v2

- `View()` returns `tea.View`, not `string`: `v := tea.NewView(s)`.
- Terminal modes are **fields on the view**, set on every render: `v.AltScreen = true`, `v.MouseMode = tea.MouseModeCellMotion`. They used to be program options or commands.
- **Keys:** handle `tea.KeyPressMsg`; `tea.KeyMsg` is now an interface covering presses and releases.
  - `msg.String()` gives `"q"`, `"ctrl+c"`, `"enter"`, `"left"`, and so on. **Space is `"space"`**, not `" "`.
  - Fields: `Code` (a rune, or `tea.KeyEnter` etc.), `Text` (string), `Mod` (check with `msg.Mod.Contains(tea.ModCtrl)`).
  - In tests, send `tea.KeyPressMsg{Code: 'q', Text: "q"}`; for space, `{Code: ' ', Text: " "}`.
- **Mouse:** `tea.MouseMsg` is an interface; call `msg.Mouse()` for X, Y and the button. Wheel events are `tea.MouseWheelMsg`.
- **Background colour:** return `tea.RequestBackgroundColor` from `Init`. `tea.BackgroundColorMsg` then arrives, and `msg.IsDark()` feeds `lipgloss.LightDark(isDark)`.
- **Commands:** `tea.Tick(d, fn)`, `tea.Batch`, `tea.Sequence`, `tea.Quit`. `tea.RequestWindowSize` is a `Msg`, not a `Cmd`.
- **Options:** `tea.WithContext`, `tea.WithInput`, `tea.WithOutput`, `tea.WithWindowSize(w, h)` (useful in tests), `tea.WithColorProfile`.
- `p.Run()` returns `(tea.Model, error)`.

## Lip Gloss v2

- `lipgloss.Color("#hex")` is a **function** that returns `color.Color`. `AdaptiveColor` is gone; use `lipgloss.LightDark(isDark)(light, dark)` instead. Bubbles' default styles also take `isDark`.
- **No global renderer:** `Style.Render` always emits full-colour ANSI, and downsampling happens on output.
  - In Bubble Tea it happens automatically.
  - In plain CLI output, print through `lipgloss.Println`, `lipgloss.Fprintln` or `lipgloss.Sprint`. These follow the stdout colour profile and honour `NO_COLOR`; printing `Render` output with `fmt` does not.
- Outside Bubble Tea, query the background explicitly with `lipgloss.HasDarkBackground(os.Stdin, os.Stdout)`.
- `lipgloss.Width` and `lipgloss.Height` measure display width and are CJK-safe. `JoinHorizontal`, `JoinVertical` and `Place` work as before.
- Layers and a compositor (`NewLayer`, `NewCompositor`) are available for overlays (help, moveset panel).

## Bubbles v2

- Width and height are methods, not fields: `SetWidth(n)` and `Width()`.
- Constructors are `New(...)` only: `list.New(items, delegate, w, h)`, `table.New(opts...)`, `viewport.New(opts...)`, `help.New()`, `spinner.New(opts...)`.
- Key bindings: `key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous day"))`, checked with `key.Matches(msg, binding)`.

## Fang

- `fang.Execute(ctx, rootCmd, fang.WithVersion(v), fang.WithCommit(c))` adds styled help and errors, `--version`, and `man` and `completion` commands.

## teatest v2

- `tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(w, h))`, then `tm.Send(msg)`, `tm.Type("text")`, `tm.Quit()`.
- `tm.FinalModel(t, teatest.WithFinalTimeout(d))` and `tm.FinalOutput(t)` both need the model to quit.
- `teatest.WaitFor(t, tm.Output(), func([]byte) bool, …)` waits for an intermediate state.
- `teatest.RequireEqualOutput(t, out)` compares against a golden file in `testdata/`; update it with `-update`.
