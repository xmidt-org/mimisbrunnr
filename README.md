# Mimisbrunnr

(Pronounced "mims-brun-er")

[![Build Status](https://travis-ci.com/xmidt-org/mimisbrunnr.svg?branch=master)](https://travis-ci.com/xmidt-org/mimisbrunnr)
[![codecov.io](http://codecov.io/github/xmidt-org/mimisbrunnr/coverage.svg?branch=master)](http://codecov.io/github/xmidt-org/mimisbrunnr?branch=master)
[![Code Climate](https://codeclimate.com/github/xmidt-org/mimisbrunnr/badges/gpa.svg)](https://codeclimate.com/github/xmidt-org/mimisbrunnr)
[![Issue Count](https://codeclimate.com/github/xmidt-org/mimisbrunnr/badges/issue_count.svg)](https://codeclimate.com/github/xmidt-org/mimisbrunnr)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/mimisbrunnr)](https://goreportcard.com/report/github.com/xmidt-org/mimisbrunnr)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/mimisbrunnr/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/xmidt-org/mimisbrunnr.svg)](CHANGELOG.md)
[![GoDoc](https://godoc.org/github.com/xmidt-org/mimisbrunnr?status.svg)](https://godoc.org/github.com/xmidt-org/mimisbrunnr)

## Summary

Mimisbrunnr provides device level event delivery.  It registers a webhook to Caduceus.  Upon receiving Caduceus events, Mismisbrunnr fans out those events to any current device registrations it has.  Mismisbrunnr has an API to allow consumers to register to receive events for a specific device.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Details](#details)
- [Install](#install)
- [Contributing](#contributing)

## Code of Conduct

This project and everyone participating in it are governed by the [XMiDT Code Of Conduct](https://xmidt.io/code_of_conduct/). 
By participating, you agree to this Code.

## Details

This service is still being developed, so some details are currently unknown.  This is what we know:

A consumer will request events from a particular device to be sent to a destination.
The request MUST contain the following information:
- deviceID
- destination and required information to successfully send the message such as auth or access keys.
- duration for how long to listen to the event. (details of this still being discussed)
- a filter function to determine if the event should be sent.
For the remainder of that duration, events will be delivered to the destination
specified with best effort.(aka not guaranteed)

### Additional Design Decisions:
- All incoming and outgoing messages must be WRP
- Duration will have a maximum time that consumers can listen to aka 1 hour
- Destinations for the mvp will only support http and sqs
- An internal buffering system will be used, one per destination.
  (as opposed to the key (deviceID+destination))
- If the channel for the destination is full, the destinations will be notified
and message will be dropped

## Install

Add details here.

## Contributing

Refer to [CONTRIBUTING.md](CONTRIBUTING.md).
