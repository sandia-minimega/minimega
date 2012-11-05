#!/bin/bash

source env.bash

# testing 
echo TESTING
for i in `ls src`
do
	go test $i || exit 1
done
echo

# build packages
echo BUILD PACKAGES
for i in `ls src`
do
	echo $i
	go install $i || exit 1
done
echo

# list bugs
echo BUGS
grep -rHsi '//[ ]*BUG' src
echo

# list todos
echo TODO
grep -rHsi '//[ ]*TODO' src
echo
