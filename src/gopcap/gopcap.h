/*
 * Copyright (2013) Sandia Corporation.
 * Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
 * the U.S. Government retains certain rights in this software.
 */

#ifndef GOPCAP
#define GOPCAP

#include <pcap.h>

pcap_t *gopcapInit(char *dev);
pcap_dumper_t *gopcapPrepare(pcap_t *dev, char *filename);
void gopcapCapture(pcap_t *dev, pcap_dumper_t *handle);
int gopcapClose(void *handle, void *dumper_handle);

#endif
