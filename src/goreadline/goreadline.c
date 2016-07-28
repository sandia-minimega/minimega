#include <stdio.h>
#include <unistd.h>
#include <sys/select.h>
#include <sys/time.h>
#include <readline/readline.h>
#include <signal.h>

volatile int abort_getc = 0;

int maybe_getc(FILE *stream) {
	int result;
	unsigned char c;

	fd_set rfds;
	struct timeval tv;

	while(!abort_getc) {
		FD_ZERO(&rfds);
		FD_SET(fileno(stream), &rfds);

		// try to read every 0.1 seconds
		tv.tv_sec = 0;
		tv.tv_usec = 100000;

		result = select(fileno(stream)+1, &rfds, NULL, NULL, &tv);

		if(result == -1){
			return EOF;
		} else if(result) {
			result = read(fileno(stream), &c, sizeof(unsigned char));
			if(result == sizeof(unsigned char)) {
				return c;
			}

			return EOF;
		}
	}

	abort_getc = 0;

	// ESC kills the history search
	return ESC;
}

int mini_redisplay(void) {
	// We only want to call this once per signal
	rl_event_hook = NULL;

	// This will free any partial state associated with the current input line
	// (undo information, any partial history entry, any partially-entered
	// keyboard macro, and any partially-entered numeric argument). This should
	// be called before rl_cleanup_after_signal(). The Readline signal handler
	// for SIGINT calls this to abort the current input line.
	rl_free_line_state();

	// This function will reset the state of the terminal to what it was before
	// readline() was called, and remove the Readline signal handlers for all
	// signals, depending on the values of rl_catch_signals and
	// rl_catch_sigwinch.
	rl_cleanup_after_signal();

	// This will reinitialize the terminal and reinstall any Readline signal
	// handlers, depending on the values of rl_catch_signals and
	// rl_catch_sigwinch.
	rl_reset_after_signal();

	// If an application wishes to install its own signal handlers, but still
	// have readline display characters that generate signals, calling this
	// function with sig set to SIGINT, SIGQUIT, or SIGTSTP will display the
	// character generating that signal.
	rl_echo_signal_char(SIGINT);

	// Move the cursor to the start of the next screen line.
	rl_crlf();

	// Clear text that has been input so far
	*rl_line_buffer = 0;
	rl_point = 0;
	rl_end = 0;
	rl_mark = 0;

	rl_reset_line_state();

	rl_redisplay();
}
