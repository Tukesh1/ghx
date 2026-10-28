# Record the README demo GIF with VHS (https://github.com/charmbracelet/vhs):
#   go build -o ghx . && vhs docs/demo.tape
Output docs/demo.gif

Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 14
Set Width 1600
Set Height 940
Set Padding 16
Set Theme "GitHub Dark"
Set TypingSpeed 60ms

Hide
Type "./ghx --demo" Enter
Sleep 1.5s
Show

Sleep 2s
Type "jj" Sleep 600ms
Tab Sleep 800ms
Type "jj" Sleep 600ms
Enter Sleep 2s
Type "1" Sleep 2s
Type "j" Sleep 600ms
Type "3" Sleep 1.5s
Type "j" Sleep 600ms Type "j" Sleep 1s
Type "a" Sleep 1.5s
Type "5" Sleep 1.5s
Enter Sleep 3s
Escape Sleep 800ms
Type "6" Sleep 1s
Type "c" Sleep 600ms
Type "Repro confirmed on main — fix incoming in #1838 🚀" Sleep 800ms
Ctrl+S Sleep 1.5s
Type "n" Sleep 1.5s
Type "jj" Sleep 800ms
Escape Sleep 600ms
Type "t" Sleep 1.2s
Type "t" Sleep 1.2s
Type "/orbit" Sleep 1s
Enter Sleep 1.5s
Type "q"
