/*
 * Copyright (2014) Sandia Corporation.
 * Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
 * the U.S. Government retains certain rights in this software.
 */

#include <stdlib.h>
#include <stdio.h>
#include <pcap.h>

#include "pcap.h"

pcap_t *gopcapInit(char *dev) {
	pcap_t *handle;			/* session handle */
	char errbuf[PCAP_ERRBUF_SIZE]; 	/* error string */

	handle = pcap_open_live(dev, BUFSIZ, 1, 1000, errbuf);
	return handle;
}

pcap_dumper_t *gopcapPrepare(pcap_t *dev, char *filename) {
	pcap_dumper_t *handle;
	handle = pcap_dump_open(dev, filename);
	return handle;
}

void gopcapCapture(pcap_t *dev, pcap_dumper_t *handle) {
	pcap_loop(dev, 0, &pcap_dump, (u_char *)handle);
}

int gopcapClose(void *handle, void *dumper_handle) {
	if (handle == NULL) {
		return -1;
	}

	if (dumper_handle != NULL) {
		pcap_breakloop(handle);
		pcap_dump_close(dumper_handle);
	}

	pcap_close(handle);
	return 0;
}

