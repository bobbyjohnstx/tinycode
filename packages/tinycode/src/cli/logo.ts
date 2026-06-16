// Diagonal split logo: "tiny" top-left (muted), ╲ structural separator, "code" bottom-right (bright).
// splitCols[row] = x position of ╲ in that row; chars at or after splitCol use bright/bold ink.
export const logo = {
  rows: [
    "▀█▀ ▄_ █▀▀▄ █__█╲",
    "_█_ █_ █__█ _▀▀█  ╲",
    "_▀_ ▀_ ▀~~▀ ___▀    ╲    █▀▀▀ █▀▀█ █▀▀█ █▀▀█",
    "                      ╲  █___ █__█ █__█ █^^^",
    "                        ╲▀▀▀▀ ▀▀▀▀ ▀▀▀▀ ▀▀▀▀",
  ],
  splitCols: [16, 18, 20, 22, 24],
}

export const go = logo

export const marks = "_^~,"
