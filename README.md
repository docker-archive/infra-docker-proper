# Docker Proper
*OS. Proper swabbing the deck*

This tool will delete all containers that are:

- not running
- exit time is older than `-ca` hours ago
- not used as volume containers by containers created after `-ca` hours ago

And will attempt(!) to delete *all* images that are:

- creation time is older than `-ia` hours ago
- not used by any container created after `-ca` hours ago

If unsure, use `-dry` before running it.

**THERE IS NO CONFIRMATION. IF YOU RUN THIS TOOL, IT WILL DELETE
STUFF. YOU HAVE BEEN WARNED.**

## Usage

    Usage of ./docker-proper:
      -a="unix:///var/run/docker.sock": address of docker daemon
      -ca=672h0m0s: Max container age
      -dry=false: Dry run; do not actually delete
      -ia=672h0m0s: Max images age
      -r=0: Run continously in given interval
      -v=false: Be verbose


## Docker image
I dockerize all the things, also this thing.

You can use it to remove stuff from local machine by bind-mounting the socket:

    $ docker run --rm -v /var/run/docker.sock:/docker.sock fish/docker-proper -a unix:///docker.sock

Or just specify a remote host by using `-a`.

You can also just use the image to pull out the docker-proper binary like this:

    $ docker run --rm fish/docker-proper --entrypoint cat /docker-proper/docker-proper > docker-proper

If you specify `-r 24h` the docker-proper and the container will sleep
24 hours and then repeat the cleanup. This is useful if you don't want
to create a cronjob for this.
