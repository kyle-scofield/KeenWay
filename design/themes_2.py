THEMES = {
    'j-sunset': ('J · Закат', 'Тёплый градиент оранжевого и розового на тёмно-фиолетовом. Эмоционально и тепло.', '''
body { color: #fff4ec; background: linear-gradient(160deg, #2a1033, #1a0b24 60%); }
nav { background: #ffffff08; } .brand b { color: #ff8a5b; }
nav a { border-radius: 12px; color: #e9c9d6aa; } nav a.on { background: linear-gradient(90deg, #ff6b6b33, #ffb34733); color: #fff; }
.hero { background: #ffffff0d; border-radius: 26px; border: 1px solid #ffffff1a; }
.ring { background: linear-gradient(160deg, #ffb347, #ff6b6b 50%, #c2185b); box-shadow: 0 10px 50px #ff6b6b66; }
.modes { background: #00000033; border-radius: 99px; } .modes button { border-radius: 99px; color: #f3d7e2; } .modes .on { background: linear-gradient(90deg, #ffb347, #ff6b6b); color: #2a1033; font-weight: 700; }
.bar { background: #ffffff1a; border-radius: 99px; } .bar i { background: linear-gradient(90deg, #ffb347, #ff6b6b); border-radius: 99px; }
.tile { background: #ffffff0d; border-radius: 18px; border: 1px solid #ffffff14; } .tile em { color: #ffb347; } .tile.off em { color: #b993a6; }
'''),
    'k-bento': ('K · Бенто', 'Тёмная сетка из карточек разного размера, как на презентациях Apple. Всё на одном экране.', '''
body { color: #f5f5f7; background: #000; }
nav { background: #0c0c0d; } .brand b { color: #2997ff; }
nav a { border-radius: 10px; color: #86868b; } nav a.on { background: #1d1d1f; color: #f5f5f7; }
main { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; align-content: start; }
h1 { grid-column: 1 / -1; font-size: 32px; margin: 0; }
.hero { grid-column: 1 / -1; background: #1d1d1f; border-radius: 28px; }
.ring { background: radial-gradient(circle, #30d158 0 52%, #30d15833 53% 100%); color: #000; }
.ring small { color: #000; }
.modes { background: #000; border-radius: 12px; } .modes button { border-radius: 9px; color: #a1a1a6; } .modes .on { background: #2997ff; color: #fff; }
.bar { background: #333; border-radius: 99px; } .bar i { background: #2997ff; border-radius: 99px; }
.tiles { display: contents; }
.tile { background: #1d1d1f; border-radius: 22px; padding: 20px; } .tile:first-child { grid-column: span 2; } .tile b { font-size: 18px; }
.tile em { color: #30d158; } .tile.off em { color: #86868b; }
@media (max-width: 760px) { main { grid-template-columns: 1fr 1fr; } }
'''),
    'l-retro': ('L · Ретро Windows 98', 'Серые окна, объёмные кнопки, синий заголовок. Ностальгия и шутка — запоминается.', '''
body { color: #000; background: #008080; font-family: Tahoma, "MS Sans Serif", Verdana, sans-serif; font-size: 14px; }
nav { background: #c0c0c0; border-right: 2px solid #000; box-shadow: inset -1px -1px #808080, inset 1px 1px #fff; }
.brand { background: linear-gradient(90deg, #000080, #1084d0); color: #fff; padding: 6px 10px; font-size: 16px; } .brand b { color: #fff; }
nav a { color: #000; } nav a.on { background: #000080; color: #fff; }
h1 { background: linear-gradient(90deg, #000080, #1084d0); color: #fff; font-size: 16px; padding: 4px 10px; margin: 0; }
.hero { background: #c0c0c0; box-shadow: inset -2px -2px #808080, inset 2px 2px #fff; margin-bottom: 0; }
.ring { border-radius: 0; background: #fff; box-shadow: inset 2px 2px #808080, inset -2px -2px #fff; color: #008000; }
.modes { padding: 0; gap: 6px; } .modes button { background: #c0c0c0; box-shadow: inset -2px -2px #808080, inset 2px 2px #fff; }
.modes .on { box-shadow: inset 2px 2px #808080, inset -2px -2px #fff; background: #000080; color: #fff; font-weight: 700; }
.bar { background: #fff; height: 18px; box-shadow: inset 2px 2px #808080; padding: 2px; } .bar i { background: repeating-linear-gradient(90deg, #000080 0 10px, transparent 10px 12px); }
.tile { background: #c0c0c0; box-shadow: inset -2px -2px #808080, inset 2px 2px #fff; } .tile em { color: #008000; } .tile.off em { color: #606060; }
'''),
    'm-ocean': ('M · Океан (дашборд)', 'Тёмно-бирюзовый, как панель мониторинга: акцент на цифрах — пинг, трафик, дни.', '''
body { color: #e0f2f1; background: #031b26; }
nav { background: #052433; border-right: 1px solid #0b3a4d; } .brand b { color: #2dd4bf; }
nav a { border-radius: 8px; color: #7fb3c2; } nav a.on { background: #0b3a4d; color: #5eead4; font-weight: 600; }
.hero { background: linear-gradient(135deg, #052f40, #04212e); border: 1px solid #0b3a4d; border-radius: 16px; }
.ring { background: #031b26; border: 10px solid #14b8a6; box-shadow: 0 0 40px #14b8a644; color: #5eead4; }
.ring b { font-size: 22px; }
.big, .big2 { color: #5eead4; font-variant-numeric: tabular-nums; }
.modes { background: #031b26; border: 1px solid #0b3a4d; border-radius: 10px; } .modes button { border-radius: 7px; color: #7fb3c2; } .modes .on { background: #14b8a6; color: #031b26; font-weight: 700; }
.bar { background: #0b3a4d; border-radius: 99px; } .bar i { background: linear-gradient(90deg, #14b8a6, #38bdf8); border-radius: 99px; }
.tile { background: #052433; border: 1px solid #0b3a4d; border-radius: 12px; } .tile em { color: #2dd4bf; } .tile.off em { color: #5f8794; }
'''),
    'n-paper': ('N · Бумага', 'Тёплый бежевый, шрифт с засечками, как журнал или Notion. Спокойно и солидно.', '''
body { color: #2b2620; background: #f5f0e6; font-family: Georgia, "Times New Roman", serif; }
nav { background: #ece5d6; border-right: 1px solid #ddd3bf; font-family: system-ui, sans-serif; } .brand { font-family: Georgia, serif; } .brand b { color: #b4532a; font-style: italic; }
nav a { border-radius: 6px; color: #6b6153; } nav a.on { background: #f5f0e6; color: #2b2620; font-weight: 600; }
h1 { font-size: 36px; font-weight: 400; }
.hero { background: #fffdf8; border: 1px solid #e3d9c5; border-radius: 6px; box-shadow: 0 1px 0 #e3d9c5; }
.ring { background: #fffdf8; border: 2px solid #2b2620; color: #2e7d32; font-style: italic; }
.k { font-family: system-ui, sans-serif; }
.modes { border: 1px solid #ddd3bf; border-radius: 8px; background: #f5f0e6; font-family: system-ui, sans-serif; } .modes button { border-radius: 6px; color: #6b6153; } .modes .on { background: #2b2620; color: #f5f0e6; }
.bar { background: #ece5d6; height: 6px; } .bar i { background: #b4532a; }
.tile { background: #fffdf8; border: 1px solid #e3d9c5; border-radius: 6px; } .tile em { color: #2e7d32; font-family: system-ui, sans-serif; } .tile.off em { color: #8c8274; }
'''),
}
