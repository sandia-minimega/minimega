# create a file bar.bash in /tmp/minimega/files/ containing the following:
# #!/bin/bash
# mkdir /foo
# echo "hello cc!" >> /foo/bar.out
#
# then run the following:
cc send bar.bash
cc exec bash /tmp/miniccc/files/bar.bash
cc recv /foo/bar.out
