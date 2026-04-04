function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export class TerminalView {
  constructor(viewport, output) {
    this.viewport = viewport;
    this.output = output;
    this.maxLines = 3000;
    this.connected = false;
    this.focused = false;
    this.pendingRender = false;
    this.reset("");

    this.viewport.addEventListener("mousedown", () => {
      window.setTimeout(() => this.viewport.focus(), 0);
    });
    this.viewport.addEventListener("focus", () => {
      this.focused = true;
      this.scheduleRender();
    });
    this.viewport.addEventListener("blur", () => {
      this.focused = false;
      this.scheduleRender();
    });
  }

  setConnected(connected) {
    this.connected = Boolean(connected);
    this.scheduleRender();
  }

  reset(message = "") {
    this.lines = [[]];
    this.cursorRow = 0;
    this.cursorCol = 0;
    this.savedCursor = null;
    this.state = "normal";
    this.csiBuffer = "";

    if (message) {
      this.write(message);
      return;
    }

    this.scheduleRender();
  }

  write(text) {
    if (!text) {
      return;
    }

    for (const char of text) {
      this.consume(char);
    }

    this.trimScrollback();
    this.scheduleRender();
  }

  consume(char) {
    switch (this.state) {
      case "escape":
        this.consumeEscape(char);
        return;
      case "csi":
        this.consumeCSI(char);
        return;
      case "osc":
        this.consumeOSC(char);
        return;
      case "osc_escape":
        this.state = char === "\\" ? "normal" : "osc";
        return;
      default:
        this.consumeNormal(char);
    }
  }

  consumeNormal(char) {
    if (char === "\u001b") {
      this.state = "escape";
      return;
    }

    switch (char) {
      case "\r":
        this.cursorCol = 0;
        return;
      case "\n":
        this.cursorRow += 1;
        this.ensureLine(this.cursorRow);
        return;
      case "\b":
        this.cursorCol = Math.max(0, this.cursorCol - 1);
        return;
      case "\t":
        {
          const tabWidth = 8 - (this.cursorCol % 8);
          this.writeSpaces(tabWidth === 0 ? 8 : tabWidth);
        }
        return;
      case "\u0007":
        return;
      default:
        break;
    }

    if (char < " " || char === "\u007f") {
      return;
    }

    this.writeChar(char);
  }

  consumeEscape(char) {
    if (char === "[") {
      this.state = "csi";
      this.csiBuffer = "";
      return;
    }

    if (char === "]") {
      this.state = "osc";
      return;
    }

    switch (char) {
      case "7":
        this.saveCursor();
        break;
      case "8":
        this.restoreCursor();
        break;
      case "D":
        this.cursorRow += 1;
        this.ensureLine(this.cursorRow);
        break;
      case "E":
        this.cursorRow += 1;
        this.ensureLine(this.cursorRow);
        this.cursorCol = 0;
        break;
      case "M":
        this.cursorRow = Math.max(0, this.cursorRow - 1);
        break;
      case "c":
        this.lines = [[]];
        this.cursorRow = 0;
        this.cursorCol = 0;
        break;
      default:
        break;
    }

    this.state = "normal";
  }

  consumeCSI(char) {
    if (char >= "@" && char <= "~") {
      this.applyCSI(this.csiBuffer, char);
      this.csiBuffer = "";
      this.state = "normal";
      return;
    }

    this.csiBuffer += char;
  }

  consumeOSC(char) {
    if (char === "\u0007") {
      this.state = "normal";
      return;
    }

    if (char === "\u001b") {
      this.state = "osc_escape";
    }
  }

  applyCSI(raw, final) {
    let body = raw;
    if (body.startsWith("?") || body.startsWith(">") || body.startsWith("!")) {
      body = body.slice(1);
    }

    const params = body === ""
      ? []
      : body.split(";").map((part) => {
        const value = Number.parseInt(part || "0", 10);
        return Number.isFinite(value) ? value : 0;
      });

    const value = (index, fallback) => {
      const item = params[index];
      return item > 0 ? item : fallback;
    };

    switch (final) {
      case "A":
        this.cursorRow = Math.max(0, this.cursorRow - value(0, 1));
        break;
      case "B":
        this.cursorRow += value(0, 1);
        this.ensureLine(this.cursorRow);
        break;
      case "C":
        this.cursorCol += value(0, 1);
        break;
      case "D":
        this.cursorCol = Math.max(0, this.cursorCol - value(0, 1));
        break;
      case "E":
        this.cursorRow += value(0, 1);
        this.ensureLine(this.cursorRow);
        this.cursorCol = 0;
        break;
      case "F":
        this.cursorRow = Math.max(0, this.cursorRow - value(0, 1));
        this.cursorCol = 0;
        break;
      case "G":
        this.cursorCol = Math.max(0, value(0, 1) - 1);
        break;
      case "H":
      case "f":
        this.setCursor(value(0, 1) - 1, value(1, 1) - 1);
        break;
      case "J":
        this.eraseDisplay(params[0] ?? 0);
        break;
      case "K":
        this.eraseLine(params[0] ?? 0);
        break;
      case "P":
        this.deleteChars(value(0, 1));
        break;
      case "@":
        this.insertBlanks(value(0, 1));
        break;
      case "X":
        this.eraseChars(value(0, 1));
        break;
      case "m":
      case "h":
      case "l":
        break;
      case "s":
        this.saveCursor();
        break;
      case "u":
        this.restoreCursor();
        break;
      default:
        break;
    }
  }

  setCursor(row, col) {
    this.cursorRow = Math.max(0, row);
    this.ensureLine(this.cursorRow);
    this.cursorCol = Math.max(0, col);
  }

  saveCursor() {
    this.savedCursor = {
      row: this.cursorRow,
      col: this.cursorCol
    };
  }

  restoreCursor() {
    if (!this.savedCursor) {
      return;
    }

    this.setCursor(this.savedCursor.row, this.savedCursor.col);
  }

  writeSpaces(count) {
    for (let index = 0; index < count; index += 1) {
      this.writeChar(" ");
    }
  }

  writeChar(char) {
    const line = this.ensureLine(this.cursorRow);
    while (line.length < this.cursorCol) {
      line.push(" ");
    }

    if (this.cursorCol === line.length) {
      line.push(char);
    } else {
      line[this.cursorCol] = char;
    }

    this.cursorCol += 1;
  }

  ensureLine(row) {
    while (this.lines.length <= row) {
      this.lines.push([]);
    }
    return this.lines[row];
  }

  eraseDisplay(mode) {
    if (mode === 2) {
      this.lines = [[]];
      this.cursorRow = 0;
      this.cursorCol = 0;
      return;
    }

    if (mode === 1) {
      for (let row = 0; row < this.cursorRow; row += 1) {
        this.lines[row] = [];
      }
      this.eraseLine(1);
      return;
    }

    this.eraseLine(0);
    this.lines = this.lines.slice(0, this.cursorRow + 1);
  }

  eraseLine(mode) {
    const line = this.ensureLine(this.cursorRow);

    if (mode === 2) {
      this.lines[this.cursorRow] = [];
      return;
    }

    if (mode === 1) {
      for (let index = 0; index <= this.cursorCol && index < line.length; index += 1) {
        line[index] = " ";
      }
      return;
    }

    line.splice(this.cursorCol);
  }

  deleteChars(count) {
    const line = this.ensureLine(this.cursorRow);
    line.splice(this.cursorCol, count);
  }

  insertBlanks(count) {
    const line = this.ensureLine(this.cursorRow);
    while (line.length < this.cursorCol) {
      line.push(" ");
    }
    line.splice(this.cursorCol, 0, ...Array.from({ length: count }, () => " "));
  }

  eraseChars(count) {
    const line = this.ensureLine(this.cursorRow);
    for (let index = 0; index < count; index += 1) {
      const cursorIndex = this.cursorCol + index;
      if (cursorIndex >= line.length) {
        break;
      }
      line[cursorIndex] = " ";
    }
  }

  trimScrollback() {
    if (this.lines.length <= this.maxLines) {
      return;
    }

    const overflow = this.lines.length - this.maxLines;
    this.lines.splice(0, overflow);
    this.cursorRow = Math.max(0, this.cursorRow - overflow);
    if (this.savedCursor) {
      this.savedCursor.row = Math.max(0, this.savedCursor.row - overflow);
    }
  }

  scheduleRender() {
    if (this.pendingRender) {
      return;
    }

    this.pendingRender = true;
    window.requestAnimationFrame(() => {
      this.pendingRender = false;
      this.render();
    });
  }

  render() {
    const rawLines = this.lines.map((line) => line.join(""));
    if (!rawLines.length) {
      rawLines.push("");
    }

    const shouldAutoscroll = this.viewport.scrollTop + this.viewport.clientHeight >= this.viewport.scrollHeight - 40;
    const showCursor = this.connected && this.focused;
    const html = rawLines.map((line, index) => {
      if (!showCursor || index !== this.cursorRow) {
        return escapeHtml(line);
      }

      const padded = line.padEnd(this.cursorCol + 1, " ");
      const before = escapeHtml(padded.slice(0, this.cursorCol));
      const current = escapeHtml(padded[this.cursorCol] || " ");
      const after = escapeHtml(padded.slice(this.cursorCol + 1));
      return `${before}<span class="terminal-cursor">${current}</span>${after}`;
    }).join("\n");

    this.output.innerHTML = html;
    if (shouldAutoscroll || this.connected) {
      this.viewport.scrollTop = this.viewport.scrollHeight;
    }
  }
}
