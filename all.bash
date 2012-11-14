#!/bin/bash

source env.bash

# testing 
echo TESTING
for i in `ls src`
do
	go test $i
done
echo

# build packages
echo BUILD PACKAGES
for i in `ls src`
do
	echo $i
	go install $i
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
