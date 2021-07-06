# Raft Demo

## Start your own cluster
```shell
./raft-demo --svc 127.0.0.1:51000 --id node1 --data data/node1 --raft 127.0.0.1:52000

./raft-demo --svc 127.0.0.1:51001 --id node2 --data data/node2 --raft 127.0.0.1:52001 --join 127.0.0.1:51000

./raft-demo --svc 127.0.0.1:51002 --id node3 --data data/node3 --raft 127.0.0.1:52002 --join 127.0.0.1:51000
```

## Reference
https://github.com/Jille/raft-grpc-example
https://github.com/hanj4096/raftdb