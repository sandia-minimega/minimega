#!/bin/bash
# this scripts requires sudo

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo $SCRIPT_DIR

echo "TESTING (minitest)"

BOOT_WAIT=5
echo "starting minimega..."
sudo $SCRIPT_DIR/bin/minimega -nostdin &> /dev/null &
echo "waiting $BOOT_WAIT seconds..."
sleep $BOOT_WAIT

MINITEST_ERROR=0
echo "starting minitest..."
sudo $SCRIPT_DIR/bin/minitest
if [[ $? != 0 ]]; then
    echo -e "minitests - FAILED"
    MINITEST_ERROR=1
else
    echo -e "minitests - OKAY"
fi

QUIT_WAIT=5
echo "quitting minimega; waiting $QUIT_WAIT seconds..."
# first try a graceful shutdown of minimega
sudo $SCRIPT_DIR/bin/minimega -e quit
sleep $QUIT_WAIT
# then check if we need to force kill
if pgrep "minimega" > /dev/null
then
    echo "FAIL\tminimega still running after tests, killing process"
    pkill minimega
    MINITEST_ERROR=1
fi

echo
exit $MINITEST_ERROR
