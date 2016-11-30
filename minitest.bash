#!/bin/bash
# this script requires sudo

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo $SCRIPT_DIR

echo "TESTING (minitest)"

BOOT_WAIT=5
echo "starting minimega..."
sudo $SCRIPT_DIR/bin/minimega -nostdin &
echo "waiting $BOOT_WAIT seconds..."
sleep $BOOT_WAIT

MINITEST_ERROR=0
echo "starting minitest..."
# minitest outputs everything on STDERR, and we may want to see messages from
# both minimega and minitest (e.g. log level info) to aid in debugging
# We duplicate minitest's output from STDERR via tee and grep through it to find
# for "got != want" to detect failed tests.
MINITEST_CMD="sudo $SCRIPT_DIR/bin/minitest -level info"
MINITEST_OUTPUT="$($MINITEST_CMD 2> >(tee >(grep 'got != want') >&2) )"

if [ -n "$MINITEST_OUTPUT" ]; then
	MINITEST_ERROR=1
    echo "minitest - FAILED"
else
    echo "minitest - OKAY"
fi

QUIT_WAIT=5
echo "quitting minimega; waiting $QUIT_WAIT seconds..."
# first try a graceful shutdown of minimega
sudo $SCRIPT_DIR/bin/minimega -e quit
sleep $QUIT_WAIT
# then check if we need to force kill
if pgrep "minimega" > /dev/null
then
    echo "minimega still running after sending quit command, killing process"
    pkill minimega
    MINITEST_ERROR=1
    echo "minimega shutdown - FAILED"
else
    echo "minimega shutdown - OKAY"
fi

echo
exit $MINITEST_ERROR
