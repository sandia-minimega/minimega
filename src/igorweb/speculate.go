package main

// object conataining a single option for speculate
// an array of ten of these is passed to the client
type Speculate struct {
	// display string for "Start Time" in speculate page
	Start string
	// display string for "End Time" in speculate page
	End string
	// properly formatted start string to be used in -a tag if Reserve is
	//              clicked in speculate page
	Formatted string
}
