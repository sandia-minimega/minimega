// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

#include "_cgo_export.h"
#include "readline/readline.h"
#include <stdio.h>

char** minicli_completion(char* text, int start, int end) {
	return minicliCompletion(text, start, end);
}
