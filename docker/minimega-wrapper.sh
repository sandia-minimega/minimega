#!/bin/bash

# Check if stdout is a terminal (tty)
if [ -t 1 ]; then
    FLAGS="-it"
else
    # No TTY if piping/redirecting (clean output for completion scripts)
    FLAGS="-i"
fi

# Force no TTY for the completion flag to avoid CR characters
if [[ "$1" == "-completion" ]]; then
    FLAGS="-i"
fi

# Run the command inside the minimega container
docker exec $FLAGS minimega minimega "$@"
