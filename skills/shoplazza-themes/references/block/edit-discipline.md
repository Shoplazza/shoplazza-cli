# themes block edit discipline — how to change an existing AI card's source

- **Change only what this request names.** Leave everything else exactly as it is: don't reorder
  classes, don't touch settings nobody mentioned, don't rewrite existing ljs usage.
- **Never rewrite the whole file.** Make every change as an exact string replacement (an edit tool
  that takes the old text and the new text). A rewrite re-decides every choice in the card, so
  classes, DOM structure and ljs usage nobody asked about drift along with it.
- **Never change an existing setting's `id`.** A new id is a delete plus an add: values the
  merchant already set can no longer be read. Changing its `label`, control type or `options` is
  fine.
- **Adding a setting:** add it to `{% schema %}` `settings` and to `presets[0].settings` together,
  with a **non-empty** example value (see copy and i18n in [liquid-rules.md](liquid-rules.md)).
  The instance's initial value comes from `presets[0]`; leave it empty there and the instance gets
  an empty value.
- **Removing a setting:** remove it from `settings`, from `presets[0]` and from every liquid read,
  all three. Leftover values on instances don't matter; nothing reads them any more.
- **Find every edit point before changing anything.** Take them all from one full read of the
  source. Don't grep, and don't re-read to find line numbers — exact replacement matches text, not
  lines.
- **Each old text must be unique in the file.** When the same snippet appears in several places,
  write one replacement per place, each with enough surrounding text to tell them apart.
- **Only the exact-replacement tool changes the source** — no `sed -i`, no `awk`, no shell
  redirection over the file. An exact replacement fails loudly when its old text doesn't match;
  `sed` exits 0 when nothing matched, so the file stays unchanged, the self-check still passes, and
  the write-back ships the old card as if it were the new one.
