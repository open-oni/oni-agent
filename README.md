# ONI Agent

ONI Agent is a simple daemon you can run on an Open ONI server which runs batch
loading/purging commands when requested via a simple API over SSH.

## Basic Infos

### Simple almost to a fault

ONI Agent listens on a configured port for ssh connections and runs a batch
load or purge command based on the request. It doesn't try to parse ONI's logs
to determine success or failure, it doesn't even try to communicate much to the
caller beyond letting the caller know we tried to do something. It's up to the
caller to use other ONI endpoints to see what's working and what isn't.

We aren't trying to provide a robust API here, just a way to run management
commands slightly less manually.

### Secure

ONI Agent uses the SSH protocol, so (if it matters) the connection is always
secure from client to server. It doesn't need an ssl certificate, because it's
SSH, not HTTPS.

But it's not sshd! ONI Agent uses the built-in Go ssh package: it does not use,
expose, or rely on a system-level ssh daemon, and does not give any kind of
shell access. You can run ONI Agent in a container that does not have sshd
installed. It needs to be able to execute the Django "manage.py" script, but
if you can't run that, you can't manage ONI anyway!

Locking down ONI Agent is easy: just use your firewall to block the port from
the public. No need for fancy auth here.

## Setup and Usage

### Service Setup

Build and run the service on a server which has the ONI codebase. For a very
simple setup, you can just export environment vars and run it directly:

```bash
export BA_BIND=":2222"
export BATCH_SOURCE="/mnt/news/production-batches"
export ONI_LOCATION="/opt/openoni/"
export HOST_KEY_FILE="/etc/oni-agent"
make
./bin/agent
```

You can also trivially set this up in systemd. **Note**: we haven't tested this
exact setup just yet, so the example unit file may need tweaks!

```
[Unit]
	Description=ONI Agent
	After=network.target
	RequiresMountsFor=/mnt/news

[Service]
  Environment="BA_BIND=:2222"
  Environment="BATCH_SOURCE=/mnt/news/production-batches"
  Environment="ONI_LOCATION=/opt/openoni/"
  Environment="HOST_KEY_FILE=/etc/oni-agent"
	Type=simple
	ExecStart=/usr/local/oni-agent/agent
	SyslogIdentifier=oni-agent

[Install]
	WantedBy=multi-user.target
```

### Usage

Normally you'd use an app like [NCA][nca] to automate connectivity, but manual
invocation using a standard Linux ssh client is also fine, and can still save
time compared to remembering all the commands necessary, changing to the right
directory, etc.

```bash
# Purge version 1 of "myankeny" and load version 2
ssh -p2222 nobody@your.oni.host -C "purge-batch 'batch_oru_myankeny_ver01'"
ssh -p2222 nobody@your.oni.host -C "load-batch 'batch_oru_myankeny_ver02'"
```

The username doesn't matter: ONI Agent doesn't use this for anything. There is
no password, no ssh keys to worry about, etc. The *connection* is secure, but
it's up to you to keep the port locked down to internal connections.

[nca]: <https://github.com/uoregon-libraries/newspaper-curation-app>

## Why?

Open ONI currently has only web listeners which are proxied from Apache, or CLI
management commands that have to be invoked manually by opening a shell on a
server / in a container. Adding REST endpoints has proven more complex than
expected, because loading large batches can several minutes. In rare cases,
huge batches can even take an hour to load. In HTTP-land, this is an eternity.

A client can't expect to hold a connection that long, and continuing the REST
request after disconnect takes a bit of tomfoolery that starts to get brittle.
Then there's Apache reaping processes semi-randomly when RAM starts to balloon,
state needing to be held in a new ONI database table, new endpoints to check if
a long-running process is done, the complexity of locking down REST endpoints
to only authorized connections, ....

A true background job runner quickly proved to be out of scope for our
timeline, and remotely connecting to a real bash shell was not only risky, but
difficult for containerized setups where sshd is not likely to be installed.
Hence the creation of ONI Agent, a hacky but dependable stop-gap solution to
all of life's little problems.
