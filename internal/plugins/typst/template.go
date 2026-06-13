package typst

import (
	"bytes"
	"text/template"
)

// InteriorPage represents the data for a single page in the book.
type InteriorPage struct {
	Text      string
	ImagePath string
}

// InteriorParams holds the parameters for the interior layout template.
type InteriorParams struct {
	PhysicalWidth  float64
	PhysicalHeight float64
	MarginTop      float64
	MarginBottom   float64
	MarginInside   float64
	MarginOutside  float64
	FontFamily     string
	Pages          []InteriorPage
}

// CoverParams holds the parameters for the cover layout template.
type CoverParams struct {
	PhysicalWidth     float64
	PhysicalHeight    float64
	BackCoverWidth    float64
	SpineWidth        float64
	SpineTextEligible bool
	FrontCoverDx      float64
	FrontCoverWidth   float64
	FrontImagePath    string
	Title             string
	Subtitle          string
	Author            string
	FontFamily        string
}

const interiorTemplateSrc = `#set page(
  width: {{printf "%.4f" .PhysicalWidth}}in,
  height: {{printf "%.4f" .PhysicalHeight}}in,
  margin: (
    top: {{printf "%.4f" .MarginTop}}in,
    bottom: {{printf "%.4f" .MarginBottom}}in,
    inside: {{printf "%.4f" .MarginInside}}in,
    outside: {{printf "%.4f" .MarginOutside}}in,
  )
)
#set text(font: "{{.FontFamily}}", size: 18pt, fill: white)

{{range $page := .Pages}}
{{if $page.ImagePath}}
#page(background: image("{{$page.ImagePath}}", width: 100%, height: 100%, fit: "cover"))[
  #align(bottom + center)[
    #block(
      fill: rgb(0, 0, 0, 180),
      inset: 15pt,
      radius: 8pt,
      width: 90%,
    )[
      #align(center)[
        {{$page.Text}}
      ]
    ]
  ]
]
{{else}}
#page(background: none)[
  #set text(fill: black)
  #align(center + horizon)[
    #text(size: 24pt, weight: "bold")[
      {{$page.Text}}
    ]
  ]
]
{{end}}
{{end}}
`

const coverTemplateSrc = `#set page(
  width: {{printf "%.4f" .PhysicalWidth}}in,
  height: {{printf "%.4f" .PhysicalHeight}}in,
  margin: (top: 0in, bottom: 0in, left: 0in, right: 0in)
)

#box(width: 100%, height: 100%, fill: rgb("#111111"))[
  #place(top + left, dx: 0in, dy: 0in)[
    #box(width: {{printf "%.4f" .BackCoverWidth}}in, height: 100%, fill: rgb("#1b1b1f"))[
      #align(center + horizon)[
        #text(font: "{{.FontFamily}}", size: 14pt, fill: rgb("#cccccc"))[
          {{.Title}}
        ]
      ]
    ]
  ]
  
  #place(top + left, dx: {{printf "%.4f" .BackCoverWidth}}in, dy: 0in)[
    #box(width: {{printf "%.4f" .SpineWidth}}in, height: 100%, fill: rgb("#151518"))[
      {{if .SpineTextEligible}}
      #align(center + horizon)[
        #rotate(-90deg)[
          #text(font: "{{.FontFamily}}", size: 10pt, fill: white)[{{.Title}} - {{.Author}}]
        ]
      ]
      {{end}}
    ]
  ]
  
  #place(top + left, dx: {{printf "%.4f" .FrontCoverDx}}in, dy: 0in)[
    #box(width: {{printf "%.4f" .FrontCoverWidth}}in, height: 100%)[
      {{if .FrontImagePath}}
      #image("{{.FrontImagePath}}", width: 100%, height: 100%, fit: "cover")
      {{end}}
      #place(top + center, dy: 1.5in)[
        #block(
          width: 80%,
          fill: rgb(0, 0, 0, 180),
          inset: 20pt,
          radius: 10pt,
        )[
          #align(center)[
            #text(font: "{{.FontFamily}}", size: 28pt, weight: "bold", fill: white)[{{.Title}}]
            #v(10pt)
            #text(font: "{{.FontFamily}}", size: 16pt, style: "italic", fill: rgb("#dddddd"))[{{.Subtitle}}]
            #v(20pt)
            #text(font: "{{.FontFamily}}", size: 14pt, weight: "medium", fill: white)[{{.Author}}]
          ]
        ]
      ]
    ]
  ]
]
`

var (
	interiorTemplate = template.Must(template.New("interior").Parse(interiorTemplateSrc))
	coverTemplate    = template.Must(template.New("cover").Parse(coverTemplateSrc))
)

// RenderInterior renders the interior Typst markup from params.
func RenderInterior(params InteriorParams) (string, error) {
	var buf bytes.Buffer
	if err := interiorTemplate.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderCover renders the cover Typst markup from params.
func RenderCover(params CoverParams) (string, error) {
	var buf bytes.Buffer
	if err := coverTemplate.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}
