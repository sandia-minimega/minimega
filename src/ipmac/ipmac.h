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
