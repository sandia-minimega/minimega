#!/bin/bash

bash checkformat.bash
bash vet.bash
# bash lint.bash
bash build.bash
bash test.bash
# bash minitest.bash
bash doc.bash
