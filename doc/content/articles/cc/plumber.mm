# start a shell on VM bar with stdin and stdout attached to named pipes;
# anything written to in_pipe reaches the shell, its output lands on out_pipe
cc filter name=bar
cc background stdout=out_pipe stdin=in_pipe cmd.exe
pipe in_pipe "dir C:\\"
