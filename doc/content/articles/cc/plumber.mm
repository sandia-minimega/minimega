# start a shell on the VM workstation with stdin and stdout attached to named
# pipes; anything written to shell_in reaches the shell, its output lands on
# shell_out
cc filter name=workstation
cc background stdout=shell_out stdin=shell_in cmd.exe
# the client starts the shell on its next check-in, and a write with no
# reader is discarded, so wait until the shell reads shell_in
shell timeout 60 sh -c "until minimega -e .annotate false .columns name,readers pipe | grep -q 'shell_in *| *1'; do sleep 1; done"
pipe shell_in "dir C:\\"
