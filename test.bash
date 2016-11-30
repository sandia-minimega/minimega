#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

. $SCRIPT_DIR/env.bash

. $SCRIPT_DIR/addversion.bash

# testing
echo TESTING
for i in `ls $SCRIPT_DIR/src | grep -v vendor`
do
	go test $i
done

# test python bindings
$SCRIPT_DIR/misc/python/test_minimega.py &> /dev/null
if [[ $? != 0 ]]; then
    echo -e "FAIL\tminimega.py"
else
    echo -e "ok\tminimega.py"
fi
echo
