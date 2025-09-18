# ONI Agent

ONI Agent is a simple daemon you can run on an Open ONI server which runs batch
loading/purging commands when requested via a simple API over SSH.

## Basic Infos

### Simple almost to a fault

ONI Agent listens on a configured port for ssh connections and runs ONI
management commands and database commands based on the request. There are a
handful of sanity checks to avoid unnecessary / incorrect error responses, but
generally it doesn't try to get fancy. If a command runs and ONI exits with a
non-zero status, you'll have to manually view the logs to figure out what to do
about it.

There's a basic API for getting job statuses and logs, but nothing
"intelligent" happens under the hood.

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
export DB_CONNECTION="user:password@tcp(127.0.0.1:3306)/databasename"
make
./bin/agent
```

You can also trivially set this up in systemd using
[`oni-agent.service`](oni-agent.service) as a template for your own
environment.

### Usage

Normally you'd use an app like [NCA][nca] to automate connectivity, but manual
invocation using a standard Linux ssh client is also fine, and can still save
time compared to remembering all the commands necessary, changing to the right
directory, etc.

Note that you should always consider quoting values. For most arguments to the
various agent commands, this isn't necessary, but getting in the habit will
help when you have a command where one argument is multiple words, such as in
the case of the `ensure-awardee` command.

The username doesn't matter: ONI Agent doesn't use this for anything. There is
no password, no ssh keys to worry about, etc. The *connection* is secure, but
it's up to you to keep the port locked down to internal connections.

If you are asking ONI Agent for potentially slow actions to be performed
(`load-batch` or `purge-batch`), the agent adds the jobs to a queue and runs
them one at a time in the order they were received. This ensures the server
won't become unusable in the event several huge batches are trying to load at
once.

You can use a tool like `jq` to parse the JSON responses returned by the
server. Your best bet is to look at the code to see what you can expect, but
you will generally see a job structure that contains an "id" field when a job
is created. You can use that to request more data.

Some commands normally return job ids on success, but don't always need to run
a job. If the agent determines that a job doesn't need to run, the job
structure will have an `id` field of -1. e.g.:

```json
{
  "job": {"id": -1},
  "message": "No-op: job is redundant or already completed",
  "status": "success"
}
```

[nca]: <https://github.com/uoregon-libraries/newspaper-curation-app>

### Simple Examples

A simple purge-and-reload of a batch would be something like this:

```bash
# Purge version 1 of "myankeny" and load version 2
ssh -p2222 nobody@your.oni.host "purge-batch 'batch_oru_myankeny_ver01'"
ssh -p2222 nobody@your.oni.host "load-batch 'batch_oru_myankeny_ver02'"
```

A full exchange of kicking off a job and then checking its status, using `jq`
to examine the responses, might look like this:

```
$ ssh -p2222 nobody@localhost "load-batch batch_hillsborohistoricalsociety_20240912H3MahoganyOrcoMammanBehindShrubs_ver01" | jq
{
  "job": {
    "id": 7
  },
  "message": "Job added to queue",
  "session": {
    "id": 15
  },
  "status": "success"
}

$ ssh -p2222 nobody@localhost "job-status 7" | jq
{
  "job": {
    "id": 7,
    "status": "successful"
  },
  "message": "Success: this job is complete.",
  "session": {
    "id": 17
  },
  "status": "success"
}
```

## Commands

The following commands are currently available:

- `load-title`: takes the ssh input, writes it out to a temporary location, and
  tells ONI to ingest that as a MARC XML document representing a newspaper
  title.
- `version`: reports the version number of the agent.
- `job-status <job id>`: Reports the status of the given job id: "pending",
  "started", "couldn't start", "successful", or "failed".
- `job-logs <job id>`: Reports the full list of a command's logs, with
  timestamps added for clarity.
- `load-batch <batch name>`: Creates a job to load the named batch, using the
  configured batch path combined with the batch name to find it on disk. The
  return includes a job ID for monitoring its status. A job ID of -1 indicates
  the batch doesn't need to be loaded (it's already been loaded).
- `purge-batch <batch name>`: Purges the named batch. The return includes a job
  ID for monitoring its status. If the ID is -1 it means there's no task to
  perform, most likely the batch doesn't exist, so there's nothing to purge.
- `batch-patch <original batch name> <new batch name>`: Takes one or more
  instructions from STDIN. The original batch is read and the instructions are
  carried out in a copy of that batch, `new batch name`. supported
  instructions:
  - `RemoveIssue <issue key>`: the copied batch will not have the issue
    identified by the given key. Issue keys are in the format
    `lccn/yyyymmddee`: lccn is the Library of Congress control number, `yyyy`
    is a four-digit year, `mm` is a two-digit month, `dd` is a two-digit day of
    the month, and `ee` is a two-digit edition for the issue (usually `01`).
- `ensure-awardee <MARC Org Code> <Full awardee name>`: Checks if the given
  code exists in the `core_awardee` table. If it does, success is returned. If
  it doesn't, and full awardee name was given, the awardee is created and
  success is returned. If it doesn't exist and no name was given, the agent
  will return failure.

## Development

For dev use, where you may not want to deal with integrating this with a real
ONI instance, you can use the included fake-manage script, `manage.py`. Your
environment might look something like this:

```bash
export BA_BIND=:2222
export ONI_LOCATION=$(pwd)
export BATCH_SOURCE=/path/to/oni/batches/
export HOST_KEY_FILE=$(pwd)/agent_rsa
export DB_CONNECTION="oni:oni@tcp(127.0.0.1:3306)/oni"
```

With `ONI_LOCATION` set to this project's directory, commands which would be
run against ONI's `manage.py` will use the fake management script which is just
a bash script that essentially does nothing.

You still have to run a database unfortunately, but you can just export your
ONI database's structure in a pinch and call it good enough, or use the ONI
docker setup to create a database and just run that without any actual
integration otherwise.

## Why?

Open ONI currently has only web listeners which are proxied from Apache, or CLI
management commands that have to be invoked manually by opening a shell on a
server / in a container. Adding REST endpoints has proven more complex than
expected, because loading large batches can take several minutes. In rare cases,
huge batches can even take an hour to load. In HTTP-land, this is an eternity.

An HTTP client can't expect to hold a connection that long, and continuing the
REST request after disconnect takes a bit of tomfoolery that starts to get
brittle. Then there's Apache reaping processes semi-randomly when RAM starts to
balloon, state needing to be held in a new ONI database table, new endpoints to
check if a long-running process is done, the complexity of locking down REST
endpoints to only authorized connections, ....

A true background job runner quickly proved to be out of scope for our
timeline, and remotely connecting to a real bash shell was not only risky, but
difficult for containerized setups where sshd is not likely to be installed.
Hence the creation of ONI Agent, a hacky but dependable stop-gap solution to
all of life's little problems.
