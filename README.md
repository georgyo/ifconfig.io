
# ifconfig.io
[![Build Status](https://drone.io/github.com/georgyo/ifconfig.io/status.png)](https://drone.io/github.com/georgyo/ifconfig.io/latest)

Inspired by ifconfig.me, but designed for pure speed. A single server can do 18,000 requests per seconds while only consuming 50megs of ram.

I used the gin framework as it does several things to ensure that there are no memory allocations on each request, keeping the GC happy and preventing unnessary allocations.

Tested to handle 15,000 requests persecond on modest hardware with an average response time of 130ms.
![LoadTest](http://i.imgur.com/xgR4u1e.png)
