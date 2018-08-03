# TICK stack 

With Grafana in place of Chronograf

Get the stack (only once):

```
git clone https://github.com/nicolargo/docker-influxdb-grafana.git
cd docker-influxdb-grafana
```

Run your stack:

```
docker-compose up

```

Install plugin:

```
docker exec -it grafana bash
grafana-cli --pluginUrl https://github.com/ilgizar/ilgizar-candlestick-panel/archive/master.zip plugins install candlestick-panel
```
