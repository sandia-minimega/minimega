#!/bin/bash

echo "BUILD DOCUMENTATION"

set -e

# build api documentation
echo "apigen"
bin/apigen -bin bin/minimega \
            -template doc/content_templates/minimega_api.template \
            -sections .,mesh,vm,host \
            > doc/content/articles/api.article

bin/apigen -bin bin/minirouter \
            -template doc/content_templates/minirouter_api.template \
            -sections . \
            > doc/content/articles/minirouter_api.article
