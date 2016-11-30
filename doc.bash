#!/bin/bash

echo "BUILD DOCUMENTATION"

# build api documentation
echo "apigen"
bin/apigen > doc/content/articles/api.article
exit $?
