# Redis Proxy Exporter


## Building and running the exporter

### Build and run locally

```sh
git clone https://github.com/vegazmaoning/redis_proxy_exporter.git
cd redis_proxy_exporter
make
./predixy_exporter --help
```


### Command line flags

| Name           | Environment Variable Name | Description                                                  |
| -------------- | ------------------------- | ------------------------------------------------------------ |
| proxy.addr     | PROXY_ADDR                | Address of the Redis  Proxy instance, defaults to  localhost:6379`. |
| proxy.password | PROXY_PASSWORD            | Password of the Redis  Proxy , defaults to `""` (no password). |
| timeout        |                           | Timeout for connection to Redis Proxy , defaults to "15s" (in Golang duration format) |

## What's exported

Most items from the INFO command are exported,
see [predixy documentation](https://github.com/joyieldInc/predixy) for details.