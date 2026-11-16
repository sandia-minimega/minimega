# start a shell on the VM workstation. Named pipes connect its stdin and
# stdout. Text written to shell_in reaches the shell. The output goes to
# shell_out
cc filter name=workstation
cc background stdout=shell_out stdin=shell_in cmd.exe
# the client starts the shell on its next check-in. A write with no reader is
# discarded, so wait until the shell reads shell_in. The check reads the pipes
# of the active namespace, so make the namespace of the VM active first
shell timeout 60 sh -c "until minimega -e .annotate false .columns name,readers pipe | grep -q 'shell_in *| *1'; do sleep 1; done"
pipe shell_in "dir C:\\"
