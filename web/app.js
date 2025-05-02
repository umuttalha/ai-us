let ws, myName;
const qs = s => document.querySelector(s);

// DaisyUI renk adları → 7 tanesi yeter
const palette = ['primary','secondary','accent','info','success','warning','error'];

// Her kullanıcıya tutarlı renk ataması
const colorOf = user => palette[
  [...user].reduce((s,c)=>s+c.charCodeAt(0),0) % palette.length
];

// Odaya bağlan
qs('#find').onclick = () => {
  myName = qs('#name').value.trim() || 'anon';
  ws = new WebSocket(`ws://${location.host}/ws?name=${encodeURIComponent(myName)}`);
  ws.onopen = () => ws.send(JSON.stringify({type:'FIND_ROOM'}));

  ws.onmessage = e => {
    const m = JSON.parse(e.data);
    switch(m.type){

      case 'JOIN':
        updateCounter(m.count, m.max);
        addSys(`${m.user} odaya katıldı`);
        break;

      case 'TOPIC':
        qs('#login').classList.add('hidden');
        qs('#lobby').classList.remove('hidden');
        qs('#topic').textContent = m.text;
        qs('#waiting').classList.add('hidden');
        addSys(`== KONUMUZ: ${m.text} ==`);
        break;

      case 'CHAT':
        addMsg(m.from, m.text);
        break;

      case 'GUESS':
        addSys('Tahmin zamanı! AI kim?');
        break;
    }
  };
};

// Mesaj gönder
qs('#send').onclick = () => {
  const t = qs('#msg').value.trim();
  if(!t) return;
  ws.send(JSON.stringify({type:'CHAT',data:t}));
  qs('#msg').value='';
};

// --- UI yardımcıları ------------------------------------------------------

function updateCounter(joined,max){
  qs('#joined').textContent=joined;
  qs('#total').textContent=max;
  qs('#waiting').classList.remove('hidden');
}

function addSys(text){
  const el = document.createElement('div');
  el.className = 'text-center text-sm opacity-70';
  el.textContent = text;
  qs('#chat').append(el); scroll();
}

function addMsg(from,text){
  const mine = from===myName;
  const align = mine ? 'chat-end' : 'chat-start';
  const color = colorOf(from);

  const wrap = document.createElement('div');
  wrap.className = `chat ${align}`;

  // İsim (renkli)
  const header = document.createElement('div');
  header.className = `chat-header text-${color}`;
  header.textContent = from;
  wrap.append(header);

  // Mesaj balonu
  const bubble = document.createElement('div');
  bubble.className = `chat-bubble bg-${color} text-${color}-content`;
  bubble.textContent = text;
  wrap.append(bubble);

  qs('#chat').append(wrap); scroll();
}

function scroll(){
  const c = qs('#chat');
  c.scrollTop = c.scrollHeight;
}
