THEMES = {
    'e-glass': ('E · Матовое стекло', 'Светлое «стекло» на мягком цветном фоне, как в macOS. Воздушно и дорого.', '''
body { color: #1c1f2e; background: linear-gradient(135deg, #c7d2fe, #fbcfe8 50%, #bae6fd); }
nav { background: #ffffff55; backdrop-filter: blur(20px); border-right: 1px solid #ffffff88; }
.brand b { color: #6d28d9; }
nav a { border-radius: 12px; color: #4b5068; } nav a.on { background: #ffffffcc; color: #1c1f2e; font-weight: 600; box-shadow: 0 2px 10px #6d28d91a; }
.hero, .tile { background: #ffffff66; backdrop-filter: blur(18px); border: 1px solid #ffffffaa; border-radius: 24px; box-shadow: 0 8px 30px #6d28d914; }
.tile { border-radius: 18px; }
.ring { background: conic-gradient(#8b5cf6, #ec4899, #38bdf8, #8b5cf6); padding: 10px; } .ring > div { width: 100%; height: 100%; border-radius: 50%; background: #ffffffd9; display: grid; place-content: center; }
.modes { background: #ffffff66; border-radius: 14px; } .modes button { border-radius: 10px; color: #4b5068; } .modes .on { background: #8b5cf6; color: #fff; box-shadow: 0 4px 12px #8b5cf655; }
.bar { background: #ffffff88; border-radius: 99px; } .bar i { background: linear-gradient(90deg, #8b5cf6, #ec4899); border-radius: 99px; }
.tile em { color: #7c3aed; } .tile.off em { color: #8a8fa3; }
'''),
    'f-cyber': ('F · Киберпанк', 'Жёлтый и маджента на чёрном, острые углы, неоновые подчёркивания. Ярко и дерзко.', '''
body { color: #f4f4f5; background: #0a0a0f; font-family: "Segoe UI", system-ui, sans-serif; }
nav { background: #111118; border-right: 2px solid #fcee0a; }
.brand { color: #fcee0a; letter-spacing: .08em; text-transform: uppercase; } .brand b { color: #ff2a6d; }
nav a { color: #9ca3af; text-transform: uppercase; font-size: 13px; letter-spacing: .06em; border-left: 3px solid transparent; }
nav a.on { color: #fcee0a; border-left-color: #fcee0a; background: #fcee0a14; }
h1 { text-transform: uppercase; letter-spacing: .08em; color: #fcee0a; }
.hero { background: #14141c; border: 1px solid #ff2a6d; clip-path: polygon(0 0, calc(100% - 26px) 0, 100% 26px, 100% 100%, 26px 100%, 0 calc(100% - 26px)); }
.ring { border-radius: 0; clip-path: polygon(50% 0, 100% 25%, 100% 75%, 50% 100%, 0 75%, 0 25%); background: linear-gradient(135deg, #fcee0a, #ff2a6d); color: #0a0a0f; }
.modes { background: #0a0a0f; border: 1px solid #333; } .modes button { text-transform: uppercase; font-size: 12px; letter-spacing: .06em; color: #9ca3af; }
.modes .on { background: #fcee0a; color: #0a0a0f; font-weight: 800; }
.bar { background: #26262f; } .bar i { background: #ff2a6d; box-shadow: 0 0 12px #ff2a6d; }
.tile { background: #14141c; border: 1px solid #2a2a35; border-bottom: 2px solid #fcee0a; } .tile em { color: #fcee0a; } .tile.off { border-bottom-color: #444; } .tile.off em { color: #6b7280; }
'''),
    'g-nord': ('G · Северный (Nord)', 'Спокойные серо-голубые тона, мягкий контраст. Глаза не устают, выглядит аккуратно.', '''
body { color: #eceff4; background: #2e3440; }
nav { background: #3b4252; } .brand b { color: #88c0d0; }
nav a { border-radius: 8px; color: #d8dee9aa; } nav a.on { background: #434c5e; color: #88c0d0; font-weight: 600; }
.hero { background: #3b4252; border-radius: 16px; }
.ring { background: #2e3440; border: 8px solid #a3be8c; color: #a3be8c; }
.modes { background: #2e3440; border-radius: 10px; } .modes button { border-radius: 8px; color: #d8dee9; } .modes .on { background: #88c0d0; color: #2e3440; font-weight: 700; }
.bar { background: #2e3440; border-radius: 99px; } .bar i { background: #88c0d0; border-radius: 99px; }
.tile { background: #3b4252; border-radius: 12px; } .tile em { color: #a3be8c; } .tile.off em { color: #d8dee988; }
'''),
    'h-material': ('H · Material You', 'Как Android 14+: мягкие пастельные цвета, крупные скругления, «таблетки». Дружелюбно.', '''
body { color: #1d1b20; background: #fef7ff; font-family: Roboto, system-ui, sans-serif; }
nav { background: #f3edf7; } .brand b { color: #6750a4; }
nav a { border-radius: 99px; color: #49454f; padding: 12px 18px; } nav a.on { background: #e8def8; color: #1d1b20; font-weight: 600; }
h1 { font-weight: 400; font-size: 32px; }
.hero { background: #f3edf7; border-radius: 28px; }
.ring { background: #6750a4; color: #fff; box-shadow: 0 0 0 12px #e8def8; }
.modes { background: #e8def8; border-radius: 99px; } .modes button { border-radius: 99px; color: #49454f; } .modes .on { background: #6750a4; color: #fff; }
.bar { background: #e8def8; border-radius: 99px; } .bar i { background: #6750a4; border-radius: 99px; }
.tile { background: #f7f2fa; border-radius: 20px; border: 1px solid #e7e0ec; } .tile em { color: #6750a4; } .tile.off em { color: #79747e; }
'''),
    'i-brutal': ('I · Брутализм', 'Чёрно-белый, толстые рамки, жёсткие тени. Смело, необычно, сразу запоминается.', '''
body { color: #000; background: #fffef0; font-family: "Arial Black", "Helvetica Neue", Arial, sans-serif; }
nav { background: #fff; border-right: 4px solid #000; } .brand b { background: #ffe500; padding: 0 4px; }
nav a { color: #000; font-weight: 700; border: 3px solid transparent; } nav a.on { border-color: #000; background: #ffe500; box-shadow: 4px 4px 0 #000; }
h1 { font-size: 34px; text-transform: uppercase; }
.hero { background: #fff; border: 4px solid #000; box-shadow: 8px 8px 0 #000; }
.ring { background: #00e676; border: 4px solid #000; box-shadow: 6px 6px 0 #000; }
.modes { border: 3px solid #000; background: #fff; } .modes button { font-weight: 700; } .modes .on { background: #000; color: #ffe500; }
.bar { border: 3px solid #000; height: 16px; background: #fff; } .bar i { background: #ff3d00; }
.tile { background: #fff; border: 3px solid #000; box-shadow: 5px 5px 0 #000; } .tile em { color: #000; background: #00e676; padding: 0 4px; } .tile.off em { background: #ddd; }
'''),
}
