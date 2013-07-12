/*
 * Copyright (2012) Sandia Corporation.
 * Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
 * the U.S. Government retains certain rights in this software.
 */

#ifndef IPMAC
#define IPMAC

#include <pcap.h>

struct pair {
	char *mac;
	char *ip;
	char *ip6;
};

pcap_t *pcapInit(char *dev);
int pcapClose(void *handle);
int pcapFilter(void *handle, char *filter);
struct pair *pcapRead(void *handle);

#endif
