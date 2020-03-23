# create a root shell on the windows vm, and connect stdin and stdout to a host and a linux container
cc filter name=bar
cc background stdout=out_pipe stdin=in_pipe cmd.exe

