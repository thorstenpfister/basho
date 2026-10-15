# The README's demo GIF: make vhs (needs VHS: brew install vhs).
#
# basho runs offline on its own config, cache and state directories, so the
# recording shows the embedded snapshot only and never your settings. It
# opens Aki 2026 through the picker; whatever today's tournament is stays
# behind Hide.

Output docs/demo.gif
Require ./bin/basho

Set Shell "bash"
Set FontSize 14
Set Width 1280
Set Height 760
Set Padding 16
Set TypingSpeed 60ms
Set Framerate 30

Env XDG_CONFIG_HOME "/tmp/basho-demo/config"
Env XDG_CACHE_HOME "/tmp/basho-demo/cache"
Env XDG_STATE_HOME "/tmp/basho-demo/state"

Hide
Type "rm -rf /tmp/basho-demo && clear && ./bin/basho --offline"
Enter
Sleep 1s
# The picker, filtered to Aki 2026, then day 13.
Escape
Type "/aki 2026"
Enter 2
Type "g13"
Sleep 500ms
Show

# Day 13 with the yusho race beside it.
Sleep 2.5s
# Down to the musubi-no-ichiban and its VS screen.
Type@80ms "jjjjjjjjjjjjjjjjjj"
Sleep 800ms
Enter
Sleep 3.5s
# The moveset panel, then the bout before.
Type "m"
Sleep 2.5s
Type "m"
Type "["
Sleep 2.5s
# The standings after day 13: the yusho race, then the hoshitori grid.
Escape
Type "s"
Sleep 2.5s
Tab
Sleep 2.5s
# Spoiler-free mode hides every result not revealed yet.
Escape
Type "S"
Sleep 2.5s
Type "S"
# The help overlay.
Type "?"
Sleep 2s
Tab
Sleep 2s
Escape
Type "q"
Sleep 500ms
