A simple tool for emulating disk reading.

Run server with:

```bash
go run main.go --path '<glob>'
```

For example, for emulating randomly reading SST files in a rocks DB.
```bash
go run main.go --path '/path/to/db/*.sst'
```

POST to `/busy` for starting a workload.

```bash
# running one task, each task would read with speed of 100KB per second.
# it returns an identity in JSON like '{"id": "xxxx"}', DELETE to /busy/:id for removing the workload.
curl localhost:8081/busy -HContent-Type:application/json --data '{"n": 1, "rate_limit": "100KB"}'
```
