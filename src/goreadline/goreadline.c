#include "_cgo_export.h"
#include "readline/readline.h"
#include <stdio.h>

char** minimega_completion(char* text, int start, int end) {
	char** ret = NULL;

	if (start == 0) {
		ret = rl_completion_matches(text, (rl_compentry_func_t *)&minimegaCompletion);
	}

	return ret;
}

char** make_string_array(int len) {
	return malloc(sizeof(char*)*len);
}

void set_string_array(char** a, char* s, int i) {
	a[i] = s;
}
