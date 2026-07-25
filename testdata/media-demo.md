# glowm media demo

Run this on iTerm2, Kitty, or Ghostty to see Mermaid diagrams and Markdown
images rendered inline:

```bash
glowm testdata/media-demo.md
```

Two references in here are meant to fail, so you can see the fallback behaviour
and the warnings on stderr. That is expected.

## Mermaid diagrams

A flowchart:

```mermaid
flowchart LR
  A[Markdown file] --> B[glowm]
  B --> C{stdout is a TTY<br/>with image support?}
  C -->|yes| D[inline image]
  C -->|no| E[text fallback]
```

A sequence diagram, to confirm several diagrams render in one pass:

```mermaid
sequenceDiagram
  participant You
  participant glowm
  participant Terminal
  You->>glowm: glowm doc.md
  glowm->>glowm: rasterize diagrams and load images
  glowm->>Terminal: inline image escape sequences
  Terminal-->>You: rendered document
```

## Images

A PNG:

![a bar chart](images/bars.png)

A JPEG, which is re-encoded to PNG for the terminal:

![a colour gradient](images/gradient.jpg)

A GIF:

![concentric rings](images/target.gif)

Images and diagrams keep their document order, so this diagram appears between
the images above and below it:

```mermaid
pie title Where the bytes go
  "base64 image payload" : 78
  "rendered text" : 14
  "escape sequences" : 8
```

A 2400x1350 PNG, downscaled to the display width before encoding:

![an oversized bar chart](images/oversized.png)

## What stays as text

An image is only rendered inline when the reference is the only thing on its
line. These deliberately stay as text, because a terminal image occupies whole
rows and cannot sit inside wrapped prose or another block:

Prose around it: here is ![an inline reference](images/bars.png) mid-sentence.

- A list item: ![in a list](images/bars.png)
- Another item, for contrast

> A blockquote: ![in a blockquote](images/bars.png)

| Column |
| --- |
| ![in a table cell](images/bars.png) |

A fenced code block is left completely alone:

```markdown
![this is documentation, not an image](images/bars.png)
```

## Failure cases

A missing file, which renders as a description with a warning on stderr:

![a file that does not exist](images/nope.png)

A remote image, which is not fetched:

![a remote image](https://example.com/remote.png)

## Wrapping up

Text after all of the media, to confirm the document continues normally.
