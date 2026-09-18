# start a shell on the VM workstation with stdin and stdout attached to named
# pipes; anything written to shell_in reaches the shell, its output lands on
# shell_out
cc filter name=workstation
cc background stdout=shell_out stdin=shell_in cmd.exe
pipe shell_in "dir C:\\"
