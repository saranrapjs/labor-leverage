# labor-leverage

This houses a service, and several related packages, which power the [Labor Leverage](https://bigboy.us/labor-leverage) website.

## The service

Labor Leverage is a Go service in `cmd/server` that fetches data from the SEC's EDGAR HTML files, parsing them into structured `Fact` sets which are rendered to HTML and cached in a sqlite database. I'd initially built the service to presume all of the data being backfilled out of band, but it turns out many of the EDGAR documents are large and the total set is big enough (29GB) that it made more sense to fetch and store only where requested.

## The `ixbrl` package

Part of the implementation of this service requires parsing iXBRL-flavored XHTML documents, which is the publication format used by the SEC's EDGAR system. This package provides utilities for parsing and traversing these documents; check out the [godoc](https://pkg.go.dev/github.com/saranrapjs/labor-leverage/pkg/ixbrl) for more information.

## Future

I'd like to use a similar approach to parse and return structured financial data from IRS 990 forms for 501c3 non-proft employers!
