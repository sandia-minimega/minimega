#!/bin/bash

bash checkformat.bash
bash vet.bash
# bash lint.bash
# bash fix.bash
bash build.bash
bash test.bash
# sudo bash minitest.bash
bash doc.bash
