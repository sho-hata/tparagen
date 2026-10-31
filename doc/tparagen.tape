# Records doc/tparagen.gif. Run from the repository root:
#
#   vhs doc/tparagen.tape
#
# https://github.com/charmbracelet/vhs

Output doc/tparagen.gif

Require go
Require git

Set Shell "bash"
Set FontSize 16
Set Width 1280
Set Height 680
Set Padding 24
Set TypingSpeed 60ms

# Build tparagen and run it on a copy of doc/demo, so the demo files stay
# untouched. The global git config is ignored so that the diff looks the same
# on every machine.
Hide
Type "export PS1='$ ' GIT_CONFIG_GLOBAL=/dev/null DEMO=$(mktemp -d) && go build -o $DEMO/bin/tparagen ./cmd/tparagen && cp -R doc/demo $DEMO/src && cd $DEMO/src && export PATH=$DEMO/bin:$PATH && git init -q && git add -A && git -c user.name=demo -c user.email=demo@example.com commit -qm init && clear"
Enter
Sleep 5s
Show

Type "cat sample_test.go"
Enter
Sleep 4s

Type "clear"
Enter
Type "tparagen ./..."
Sleep 500ms
Enter
Sleep 2s

Type "git --no-pager diff"
Sleep 500ms
Enter
Sleep 6s
