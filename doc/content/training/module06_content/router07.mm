router foo route static 1.2.3.0/24 1.2.3.254

* Or to specify a default route:
router foo route static 0.0.0.0/0 1.2.3.254

* IPv6 routes are added in the same way:
router foo route static 2001:1:2:3::/64 2001:1:2:3::1
