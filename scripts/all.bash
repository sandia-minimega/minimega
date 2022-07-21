#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

bash $SCRIPT_DIR/check.bash || exit $?
bash $SCRIPT_DIR/build.bash || exit $?
bash $SCRIPT_DIR/test.bash  || exit $?
bash $SCRIPT_DIR/doc.bash   || exit $?
