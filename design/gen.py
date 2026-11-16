#!/usr/bin/env python3
"""Builds the design examples: the same «Обзор» screen, one CSS theme per file.
Themes live in themes_*.py as THEMES = {"e-glass": ("Название", "описание", "css"), ...}."""
import glob
import importlib.util
import os

HERE = os.path.dirname(os.path.abspath(__file__))

MARKUP = '''<nav>
  <div class="brand">Keen<b>Way</b></div>
  <a class="on">Обзор</a><a>Подключение</a><a>Сервисы</a><a>Свои сайты</a><a>Устройства</a><a>Серверы</a><a>Трафик</a><a>Настройки</a>
</nav>
<main>
  <h1>Обзор</h1>
  <section class="hero">
    <div class="ring"><div><b>Защищено</b><small>53 мс</small></div></div>
    <div class="info">
      <div class="k">Сервер сейчас</div>
      <div class="big">🇸🇪 Швеция 287220</div>
      <div class="modes"><button class="on">🚀 Auto-Best</button><button>🛡️ Fallback</button><button>⚙️ Manual</button></div>
      <div class="k">Подписка VPNN</div>
      <div class="big2">24 дня · 28 из 3221 ГБ</div>
      <div class="bar"><i></i></div>
    </div>
  </section>
  <section class="tiles">
    <div class="tile"><span class="ic">▶️</span><b>YouTube</b><em>через VPN</em></div>
    <div class="tile"><span class="ic">💬</span><b>Discord</b><em>через VPN</em></div>
    <div class="tile"><span class="ic">📷</span><b>Instagram</b><em>через VPN</em></div>
    <div class="tile"><span class="ic">✈️</span><b>Telegram</b><em>через VPN</em></div>
    <div class="tile off"><span class="ic">🏦</span><b>Банки</b><em>напрямую</em></div>
    <div class="tile off"><span class="ic">🎮</span><b>Игры</b><em>напрямую</em></div>
  </section>
</main>'''

# layout shared by all themes; themes change colours, shapes and type
BASE = '''* { box-sizing: border-box; }
body { margin: 0; min-height: 100vh; display: flex; font: 15px/1.5 system-ui, sans-serif; }
nav { width: 220px; flex: none; padding: 22px 14px; display: flex; flex-direction: column; gap: 3px; }
.brand { font-weight: 800; font-size: 21px; padding: 0 10px 18px; }
nav a { padding: 9px 12px; cursor: pointer; }
main { flex: 1; min-width: 0; padding: 28px 32px; }
h1 { margin: 0 0 20px; font-size: 26px; }
.hero { display: grid; grid-template-columns: 200px 1fr; gap: 28px; align-items: center; padding: 24px; }
.ring { width: 180px; height: 180px; border-radius: 50%; display: grid; place-items: center; text-align: center; }
.ring b { display: block; font-size: 19px; } .ring small { display: block; font-size: 13px; opacity: .85; }
.k { font-size: 12px; text-transform: uppercase; letter-spacing: .07em; opacity: .7; }
.big { font-size: 22px; font-weight: 700; margin: 2px 0 12px; }
.big2 { font-size: 18px; font-weight: 700; margin: 2px 0 10px; }
.modes { display: inline-flex; gap: 4px; padding: 4px; margin: 0 0 16px; }
.modes button { font: inherit; font-size: 14px; border: 0; padding: 7px 14px; cursor: pointer; background: transparent; color: inherit; }
.bar { height: 8px; overflow: hidden; } .bar i { display: block; height: 100%; width: 9%; }
.tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 12px; margin-top: 20px; }
.tile { padding: 15px; } .tile .ic { font-size: 20px; } .tile b { display: block; margin: 6px 0 1px; } .tile em { font-style: normal; font-size: 12px; font-weight: 600; }
@media (max-width: 760px) { body { flex-direction: column; } nav { width: 100%; flex-direction: row; overflow-x: auto; } .hero { grid-template-columns: 1fr; justify-items: center; } }
'''

PAGE = '''<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>KeenWay · {title}</title>
<style>
{base}
{css}
</style>
</head>
<body>
{markup}
</body>
</html>
'''


def load_themes():
    themes = {}
    for path in sorted(glob.glob(os.path.join(HERE, 'themes_*.py'))):
        spec = importlib.util.spec_from_file_location(os.path.basename(path)[:-3], path)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        themes.update(mod.THEMES)
    return themes


if __name__ == '__main__':
    themes = load_themes()
    for slug, (title, _desc, css) in themes.items():
        with open(os.path.join(HERE, f'{slug}.html'), 'w', encoding='utf-8') as f:
            f.write(PAGE.format(title=title, base=BASE, css=css.strip(), markup=MARKUP))
    print('\n'.join(themes))
