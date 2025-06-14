# Reproduce fuse deadlock that cause the init process stuck at existing state.

# The fuse filesystem will sleep 1 hour when handling read operation, we use this to simulate there is still unresponsed fuse request when init process was killed.

## Operations steps
```
CGO_ENABLED=0 go build -o loopback .
sudo docker build -t tmp:latest
sudo docker run --name=tmp  --rm --privileged  -it -v /dev/fuse:/dev/fuse tmp:latest 
# in another terminal
docker exec -it tmp sh
# wait the cat running, it will be launched after 5 second on fuse start.
ps -ef | grep cat # find the process id of the cat
# inspect it's stack and status, could found it stuck at request_wait_answer and it status is (S)sleeping.
cat /proc/${PID_OF_CAT}/stack
cat /proc/${PID_OF_CAT}/status
# after send SIGTERM to the cat,insepct its stack and status again, it still stuck at request_wait_answer but its status becomes (D) disk sleep
kill ${PID_OF_CAT}
cat /proc/${PID_OF_CAT}/status
cat /proc/${PID_OF_CAT}/stack

# Now we send SGIKILL to container, the container will become zombie and stuck at `do_exit` function.
# Because the init process is waiting child process to exit, but the child process is waiting the fuse response, the deadlock happens.

ps -ef | grep loopback
kill -9 {PID_OF_LOOPBACK}
ls /proc/${PID_OF_LOOPBACK}/task
# There is one task that stucked at `do_exit`
cat /proc/${PID_OF_LOOPBACK_THREAD}/stack
```
