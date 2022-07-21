#!/bin/bash

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"

echo "BUILD DOCUMENTATION"

set -e

# build api documentation
echo "apigen"
$ROOT_DIR/bin/apigen -bin $ROOT_DIR/bin/minimega \
            -template $ROOT_DIR/doc/content_templates/minimega_api.template \
            -sections .,mesh,vm,host \
            > $ROOT_DIR/doc/content/articles/api.article

$ROOT_DIR/bin/apigen -bin $ROOT_DIR/bin/minirouter \
            -template $ROOT_DIR/doc/content_templates/minirouter_api.template \
            -sections . \
            > $ROOT_DIR/doc/content/articles/minirouter_api.article
