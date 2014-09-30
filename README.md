# Docker Proper
*OS. Proper swabbing the deck*

This tool will delete all containers that are:

- Not running
- exited before `-ca` hours ago

And will attempt(!) to delete *all* images that were created before
`-ia` hours ago.

Docker will not remove any image that is still in use by a container,
running or not, so this should be save.

If unsure, use `-dry` before running it.

** THERE IS NO CONFIRMATION. IF YOU RUN THIS TOOL, IT WILL DELETE
STUFF. YOU HAVE BEEN WARNED.*

## Usage

    Usage of ./docker-proper:
      -a="unix:///var/run/docker.sock": address of docker daemon
      -ca=672h0m0s: Max container age
      -dry=false: Dry run; do not actually delete
      -ia=672h0m0s: Max images age
      -v=false: Be verbose

## Docker image
I dockerize all the things, also this thing.

You can use it to remove stuff from local machine by bind-mounting the socket:

    $ docker run --rm -v /var/run/docker.sock:/docker.sock fish/docker-proper -a unix:///docker.sock

Or just specify a remote host by using `-a`.

You can also just use the image to pull out the docker-proper binary like this:

    $ docker run --rm fish/docker-proper --entrypoint cat /docker-proper/docker-proper > docker-proper
