/*
 * Copyright (2013) Sandia Corporation.
 * Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
 * the U.S. Government retains certain rights in this software.
 */

#ifndef GOPCAP
#define GOPCAP

#include <pcap.h>

pcap_t *pcapInit(char *dev);
pcap_dumper_t *pcapPrepare(pcap_t *dev, char *filename);
void pcapCapture(pcap_t *dev, pcap_dumper_t *handle);
int pcapClose(void *handle, void *dumper_handle);

#endif
