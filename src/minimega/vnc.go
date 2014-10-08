package main

import ()

func cliVNC(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 4: // [record|playback] <host> <vm> <file>
		if c.Args[0] != "record" && c.Args[0] != "playback" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		/*
			host := c.Args[1]
			vm := c.Args[2]
			filename := c.Args[3]
		*/
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}
	return cliResponse{
		Error: "not yet implemented",
	}
}
