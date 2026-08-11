import { useEffect, useState } from 'react'

type Prefs = {
  defaultFormats: string[]
  language: string
  theme: 'mist' | 'ink'
}

const KEY = 'docforge_settings'

function load(): Prefs {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return JSON.parse(raw) as Prefs
  } catch {
    /* ignore */
  }
  return { defaultFormats: ['markdown', 'docx', 'json'], language: 'auto', theme: 'mist' }
}

export function SettingsPage() {
  const [prefs, setPrefs] = useState<Prefs>(load)

  useEffect(() => {
    localStorage.setItem(KEY, JSON.stringify(prefs))
    document.documentElement.dataset.theme = prefs.theme
  }, [prefs])

  function toggleFormat(f: string) {
    setPrefs((p) => ({
      ...p,
      defaultFormats: p.defaultFormats.includes(f)
        ? p.defaultFormats.filter((x) => x !== f)
        : [...p.defaultFormats, f],
    }))
  }

  return (
    <section className="page narrow">
      <div className="page-head">
        <div>
          <h1>Settings</h1>
          <p>Local preferences for this browser. Provider config stays server-side.</p>
        </div>
      </div>
      <div className="settings">
        <label>
          Theme
          <select
            value={prefs.theme}
            onChange={(e) => setPrefs({ ...prefs, theme: e.target.value as Prefs['theme'] })}
          >
            <option value="mist">Mist</option>
            <option value="ink">Ink</option>
          </select>
        </label>
        <label>
          Language preference
          <select
            value={prefs.language}
            onChange={(e) => setPrefs({ ...prefs, language: e.target.value })}
          >
            <option value="auto">Auto</option>
            <option value="en">English</option>
            <option value="vi">Vietnamese</option>
          </select>
        </label>
        <fieldset>
          <legend>Default outputs</legend>
          {['markdown', 'docx', 'json'].map((f) => (
            <label key={f} className="check">
              <input
                type="checkbox"
                checked={prefs.defaultFormats.includes(f)}
                onChange={() => toggleFormat(f)}
              />
              {f}
            </label>
          ))}
        </fieldset>
      </div>
    </section>
  )
}
