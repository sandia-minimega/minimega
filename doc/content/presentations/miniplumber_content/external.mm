# plumbing productions are similar to unix pipelines
plumb "cat data.txt" "grep -i foo" lines_with_foo

# named pipes can attach directly
plumb this is a valid production

# pipelines live until any stage of the pipeline is destroyed
plumb foo "grep -i foo" bar

# kill the above pipeline by closing foo
clear pipe foo
