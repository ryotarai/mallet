# Mallet

Mallet is a TCP tunnel that works like VPN. This depends on [jpillora/chisel](https://github.com/jpillora/chisel) for actual TCP tunneling.

## Installation

```
go get github.com/ryotarai/mallet
```

## Example Usage with SSH

Example situation:

```
Laptop --SSH--> a.example.com --> 10.0.0.0/8
```

First, install chisel to a.example.com by following https://github.com/jpillora/chisel#install

Second, launch chisel server on a.example.com and forward a port to the server:

```
$ ssh -t -L 8080:127.0.0.1:8080 a.example.com chisel server --host 127.0.0.1 --port 8080
```
(Keep this ssh process running)

Then, start Mallet:

```
$ mallet start --chisel-server http://127.0.0.1:8080 10.0.0.0/8
```
(Keep this mallet process running)

Now, all TCP traffic to 10.0.0.0/8 is forwarded via a.example.com.

## Supported Platforms

- macOS
- Linux

## Similar Projects

- https://github.com/sshuttle/sshuttle

## TODO

- Embed chisel client to reduce latency
- IPv6 support
