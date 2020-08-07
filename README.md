# Mimisbrunnr

(Pronounced "mims-brun-er")

[![Build Status](https://travis-ci.com/xmidt-org/mimisbrunnr.svg?branch=main)](https://travis-ci.com/xmidt-org/mimisbrunnr)
[![codecov.io](http://codecov.io/github/xmidt-org/mimisbrunnr/coverage.svg?branch=main)](http://codecov.io/github/xmidt-org/mimisbrunnr?branch=main)
[![Code Climate](https://codeclimate.com/github/xmidt-org/mimisbrunnr/badges/gpa.svg)](https://codeclimate.com/github/xmidt-org/mimisbrunnr)
[![Issue Count](https://codeclimate.com/github/xmidt-org/mimisbrunnr/badges/issue_count.svg)](https://codeclimate.com/github/xmidt-org/mimisbrunnr)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/mimisbrunnr)](https://goreportcard.com/report/github.com/xmidt-org/mimisbrunnr)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/mimisbrunnr/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/release/xmidt-org/mimisbrunnr.svg)](CHANGELOG.md)
[![GoDoc](https://godoc.org/github.com/xmidt-org/mimisbrunnr?status.svg)](https://godoc.org/github.com/xmidt-org/mimisbrunnr)

## Summary

Mimisbrunnr provides device level event delivery.  It registers a webhook to Caduceus.  Upon receiving Caduceus events, Mismisbrunnr fans out those events to any current device registrations it has.  Mismisbrunnr has an API to allow consumers to register to receive events for a specific device.  This registration object is called a norn.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Details](#details)
- [Install](#install)
- [Contributing](#contributing)

## Code of Conduct

This project and everyone participating in it are governed by the [XMiDT Code Of Conduct](https://xmidt.io/code_of_conduct/). 
By participating, you agree to this Code.

## Details

This service is still being developed, so some details are currently unknown. 

### API
- `POST` on `/norns` - creates the norn, stores it in argus, and returns norn id; if a norn with the 
  same destination and device id exists, it updates it as long as the client owns it)
- `PUT` on `/norns/<id>` - updates an existing norn, as long as the client owns it
- `DELETE` on `/norns/<id>` - deletes the norn from argus, as long as the client owns it
- `GET` on `/norns` - returns a list of norns that the client owns
- `GET` on `/norns/<id>` - returns the norn attached to the id given, as long as the client owns it
- `POST` on `/events` - mimisbrunnr validates the event and possibly sends it to applicable norns

### Norn
A consumer will request events from a particular device to be sent to a destination.
The request is called a Norn and MUST contain the following information:
- deviceID
- destination and required information to successfully send the message such as auth or access keys.
- duration for how long to listen to the event. (details of this still being discussed)
- a filter function to determine if the event should be sent. *Note: not a part of the MVP.*

For the remainder of that duration, events will be delivered to the destination
specified with best effort.(aka not guaranteed)

### Additional Design Decisions:
- All incoming and outgoing events must be [WRPs](https://xmidt.io/docs/wrp/overview/)
- There should be a maximum duration a norn can last, which will be configurable in the yaml.
- Destinations for the MVP will only support http and sqs
- An internal buffering system will be used, one per norn.
- No two norns will share the exact same device ID and destination.
- If the channel for the destination is full, the destinations will be notified
and message will be dropped.

## Install

Add details here.

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).
