#!/bin/bash

bash check.bash || exit $?
bash build.bash || exit $?
bash test.bash || exit $?
bash doc.bash || exit $?
