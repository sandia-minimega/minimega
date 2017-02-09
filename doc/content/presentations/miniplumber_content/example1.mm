# create a pipeline connecting foo and bar
plumb foo "sed -u s/foo/moo/" bar

plumb

# write something to foo
pipe foo "the cow says foo"

shell sleep .5

pipe

# works from the command line too
# echo "hello plumber!" | minimega -pipe foo

clear plumb
clear pipe
