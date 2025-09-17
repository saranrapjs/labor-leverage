# labor-leverage

This houses a service, and several related packages, which power the [Labor Leverage](https://bigboy.us/labor-leverage) website.

## The service

Labor Leverage is a Go service in `cmd/server` that fetches data from:

- the SEC's EDGAR HTML files
- the IRS's 990 XML files

Both data are crunched into a financial `Fact` data structure, which holds some data common to both reporting formats (e.g. number of employees), but some that are unique (e.g. stock buybacks for SEC data).

## The `facts` package

This is where a lot of the data munging happens: raw IRS and/or SEC data are transformed, mostly using elaborate traversal functions with regexes, to try to extract structured or in some cases semi or unstructured data from raw documents. The parsing is quite lossy and there are definitely corporations that will be missing one category of data or another! Yes I considered using an LLM here, but the volume of data seemed big enough that the $$$ didn't seem worth it.

## The `ixbrl` package

Part of the implementation of this service requires parsing iXBRL-flavored XHTML documents, which is the publication format used by the SEC's EDGAR system. This package provides utilities for parsing and traversing these documents; check out the [godoc](https://pkg.go.dev/github.com/saranrapjs/labor-leverage/pkg/ixbrl) for more information.

## The `edgar` package

Handles communication with the SEC's Edgar API, which is really just a specific pattern of storing and retrieving HTML files encoded with iXBLR tags.

## The `irs` package

Handles communication with the IRS' historical 990 XML filings for non-profits. These are stored in big collated zip files, but this package uses the [`cloudzip`](https://github.com/ozkatz/cloudzip) and HTTP range headers to only fetch those parts of the ZIP pertinent to the specific non-profit.

Right now some of this is hardcoded around the tax year 2024, because I'm not sure I've yet followed how or when the IRS makes a full year's returns available and/or when non-profits incrementally report their data.

## The `irsform` package

I used some XML schema to Go code generation to produce structs for each of the 990 XML files decoded by the service, but they all required hand-editing, and so that's what's in here.

## The `db` package

I'd initially built the service to presume all of the data being backfilled out of band, but it turns out many of the EDGAR documents are large and the total set is big (29GB); the service now lazily caches SEC/IRS data as time goes on in a sqlite database.

## Running locally

You'll need the Go toolchain, and [Git LFS](https://git-lfs.com). Run the server:

```shell
go run cmd/server/main.go
```

Then open http://localhost:8080/ in a browser.

## Tests

Many of the tests here are kind of worthless, and only a reflection of trying to suss out specific IRS/SEC data edge cases.

## Deployment

I manually deploy this to my server for now. Maybe I'll revisit this if other people want to contribute.
