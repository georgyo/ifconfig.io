
# ifconfig.io
[![Build Status](https://drone.io/github.com/georgyo/ifconfig.io/status.png)](https://drone.io/github.com/georgyo/ifconfig.io/latest)

Inspired by ifconfig.me, but designed for pure speed. A single server can do 18,000 requests per seconds while only consuming 50megs of ram.

I used the gin framework as it does several things to ensure that there are no memory allocations on each request, keeping the GC happy and preventing unnessary allocations.

Tested to handle 10,000 clients doing 90,000 requests persecond on modest hardware with an average response time of 42ms. Easily servicing over 5 million requests in a minute. (Updated June, 2022)

[![LoadTest](http://i.imgur.com/0vJYumD.png)](https://loader.io/reports/f1e9a7dd516ac0472351e5e0c83b0787/results/a055e51ff317cdf8a688b25e9c0e4147#response_details)
