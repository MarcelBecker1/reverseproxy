# Reverse Proxy Idea (Primegean inspired)

## General

Client -> Reverse Proxy -> Server for matchmaking -> GameServer (actual resources)

Client authenticates to proxy

Proxy finds server or starts new one

Proxy forwards auth

Server can send stuff (config) back to client via proxy

## Stuff to consider

- TCP connections (handshake, packet format)
- Filter bad actors (can be filtered by rules, but for now maybe just allow all)
- Authentication (probably also allow everyone for now)
- Server assignments
- What if Client closes connection -> Server should close connection

Need simulation to test it

## What i want to figure out

- Do we actual need one Server and then seperate smaller game entities? GameServers in normal servers doesn't really make sense


## Start with
- Just simple connection TCP (socket) client server with TCP 
- add new server (process for simulation) that has db, some config
- forward data to server through proxy

## What are we tackeling
- Load balancing
- Protection from attacks?