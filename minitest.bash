#!/bin/bash
# this script requires root

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"


if [ "$UID" -ne 0 ]; then
    echo "run as root!!!"
    exit 1
fi


MINITEST_ERROR=0


echo "TESTING (minitest)"

if [ -d "miniccc_container_rootfs" ]; then
    echo "using existing miniccc_container_rootfs/"
else
    echo "building miniccc_container.conf..."
    $SCRIPT_DIR/bin/vmbetter -branch stable -rootfs -level info $SCRIPT_DIR/misc/vmbetter_configs/miniccc_container.conf
fi
export containerfs=$SCRIPT_DIR/miniccc_container_rootfs
cp $SCRIPT_DIR/bin/miniccc $containerfs/


if [ -d "minirouter_container_rootfs" ]; then
    echo "using existing minirouter_container_rootfs/"
else
    echo "building minirouter_container.conf..."
    $SCRIPT_DIR/bin/vmbetter -branch stable -rootfs -level info $SCRIPT_DIR/misc/vmbetter_configs/minirouter_container.conf
fi
export minirouterfs=$SCRIPT_DIR/minirouter_container_rootfs
cp $SCRIPT_DIR/bin/miniccc $minirouterfs/
cp $SCRIPT_DIR/bin/minirouter $minirouterfs/


BOOT_WAIT=5
echo "starting minimega..."
$SCRIPT_DIR/bin/minimega -nostdin $@ &
echo "waiting $BOOT_WAIT seconds..."
sleep $BOOT_WAIT


echo "starting minitest..."
echo "!!! WARNING: output from both minitest and minimega may be interleaved below !!!"
# minitest outputs everything on STDERR, and we may want to see messages from
# both minimega and minitest (e.g. log level info) to aid in debugging
# We duplicate minitest's output from STDERR via tee and grep through it to find
# for "got != want" to detect failed tests.
MINITEST_CMD="$SCRIPT_DIR/bin/minitest -level info"
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
$SCRIPT_DIR/bin/minimega -e quit
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
