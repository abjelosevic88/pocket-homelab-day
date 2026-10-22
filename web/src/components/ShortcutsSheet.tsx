import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['t'], 'Today'], [['c'], 'Calendar'], [['j'], 'Journal'], [['y'], 'Year'], [['a'], 'Ask (with AI connected)'], [[','], 'Settings'], [['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Write', keys: [[['n'], "Today's entry"], [['m'], 'Add a moment to today']] },
  { title: 'On a day', keys: [[['←'], 'The day before'], [['→'], 'The day after']] },
  { title: 'Calendar', keys: [[['⇧', '←'], 'Previous month'], [['⇧', '→'], 'Next month'], [['←', '→'], 'Move the selected day'], [['↵'], 'Open the selected day']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close a dialog, menu or Settings']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
      <p className="muted small mt">In the search, type a date (2026-05-14), “yesterday” or a few words from an entry or a moment.</p>
    </Modal>
  )
}
