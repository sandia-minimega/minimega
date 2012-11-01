#!/bin/bash

source env.bash

# testing 
echo TESTING
for i in `ls src`
do
	go test $i || echo FAIL $i
done
echo

# build packages
echo BUILD PACKAGES
for i in `ls src`
do
	echo $i
	go install $i || echo FAIL $i
done
echo


