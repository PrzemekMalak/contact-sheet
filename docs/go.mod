// This go.mod only keeps docs/ (README screenshots) out of the theme's
// module zip, so sites importing the theme don't download them. Go skips
// any subdirectory that has its own go.mod when packaging a module.
module github.com/PrzemekMalak/contact-sheet/docs

go 1.22
