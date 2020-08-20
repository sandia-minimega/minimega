# miniccc programs can attach to pipes directly
# cat data.txt | miniccc -pipe foo
# miniccc -pipe bar | less

# or you can invoke pipes directly
cc exec stdin=foo some_program
cc background stdin=foo stdout=bar some_program
