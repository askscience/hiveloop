// User guide — simple language, different for each role (user / admin / superadmin), IT + EN.
// Each section: { id, icon, roles, it: [title, body], en: [title, body] }. body can be a function of the role.

const pick = (role, v) => (typeof v === "function" ? v(role) : v);
const byRole = (map) => (role) => map[role] ?? map.admin ?? map.user;

const SECTIONS = [
  {
    id: "start", icon: "user", roles: ["user", "admin", "superadmin"],
    it: ["Primi passi", byRole({
      user: `**Hiveloop** è lo spazio online della nostra organizzazione. Qui trovi il calendario di tutte le attività, le chat dei gruppi, i tuoi documenti e un assistente con intelligenza artificiale.

Sei un **utente**: puoi vedere tutto il calendario, partecipare ai gruppi, scrivere nelle chat e organizzare le attività dei gruppi di cui fai parte.

**Il menu**
- Su computer è a sinistra, sul telefono è in basso.
- :calendar: **Calendario** · :chat: **Chat** · :groups: **Gruppi** · :docs: **Documenti** · :ai: **Assistente** · :user: **Profilo**

**Usarla come un'app sul telefono**
- Vai su **Profilo → Installa l'app** (sul telefono compare anche un avviso in basso con il pulsante **Installa**).
- **iPhone (Safari):** tocca il tasto Condividi → *Aggiungi alla schermata Home*.
- **Android (Chrome):** menu :more: → *Installa app* / *Aggiungi a schermata Home*.

**Lingua e aspetto** — Dal **Profilo** scegli Italiano o English e, nella scheda **Aspetto**, il **tema** (come il telefono, chiaro o scuro), il **colore** dei pulsanti principali (10 colori o bianco e nero) e come vedere i **pulsanti** e il **menu**: icona e testo, solo icone o solo testo. Con **Dimensione (zoom)** ingrandisci o rimpicciolisci tutta l'app (dall'80% al 175%), come la scala dello schermo del computer. **Alta leggibilità** è pensata per chi vede poco: contrasto massimo, testo più grande e marcato, bordi e pulsanti più evidenti, niente animazioni; puoi unirla allo zoom. Le scelte valgono su tutti i tuoi dispositivi.

**Avatar** — Sempre dal **Profilo** premi **Crea il tuo avatar**: scegli capelli, occhi, bocca, barba, occhiali, vestiti e colori guardando l'anteprima, oppure premi **Casuale**. Il personaggio compare nelle chat, nei gruppi e nelle presenze al posto delle iniziali; con **Togli avatar** tornano le iniziali.

**Password** — Al primo accesso cambiala da **Profilo → Cambia password** (almeno 8 caratteri). Se la dimentichi, chiedi a un amministratore di reimpostarla (per ora Hiveloop non invia email).

**Registrazione** — Se non hai un account, nella pagina di accesso premi **Registrati**. Spesso un amministratore deve approvare la richiesta prima che tu possa entrare.`,
      admin: `**Hiveloop** è lo spazio online della nostra organizzazione: calendario, chat dei gruppi, documenti e assistente AI.

Sei un **amministratore (admin)**: oltre a tutto quello che fa un utente, puoi gestire **tutti** i gruppi e **tutti** gli eventi e aggiungere e rimuovere utenti.

**Il menu** — su computer a sinistra, sul telefono in basso. In più rispetto agli utenti hai :admin: **Gestione**.

**Usarla come app sul telefono** — iPhone: Condividi → *Aggiungi alla schermata Home*. Android: menu :more: → *Installa app*.

**Password** — Cambiala subito da **Profilo → Cambia password**. Gli utenti che dimenticano la password chiederanno a te: puoi reimpostarla da **Gestione**.`,
      superadmin: `**Hiveloop** è lo spazio online della nostra organizzazione: calendario, chat dei gruppi, documenti e assistente AI.

Sei il **super admin**: hai il controllo totale. Puoi fare tutto quello che fa un admin e in più **nominare o togliere admin** e **gestire gli account degli admin**. Esiste un solo super admin e nessuno può modificare o eliminare il tuo account.

**Il menu** — su computer a sinistra, sul telefono in basso. Hai anche :admin: **Gestione**.

**Sicurezza** — Usa una password lunga e unica e non condividerla. Se un admin lascia l'organizzazione, togligli il ruolo o elimina l'account da **Gestione**.`,
    })],
    en: ["Getting started", byRole({
      user: `**Hiveloop** is our organisation's online space: the calendar of all activities, group chats, your documents and an AI assistant.

You are a **user**: you can see the whole calendar, join groups, write in chats and organise the activities of the groups you belong to.

**The menu**
- On a computer it's on the left, on a phone at the bottom.
- :calendar: **Calendar** · :chat: **Chat** · :groups: **Groups** · :docs: **Documents** · :ai: **Assistant** · :user: **Profile**

**Use it like an app on your phone**
- Go to **Profile → Install the app** (on phones a notice with an **Install** button also appears at the bottom).
- **iPhone (Safari):** tap Share → *Add to Home Screen*.
- **Android (Chrome):** menu :more: → *Install app* / *Add to Home screen*.

**Language and appearance** — In **Profile** choose Italiano or English and, in the **Appearance** card, the **theme** (like the device, light or dark), the **colour** of the main buttons (10 colours or black and white) and how **buttons** and the **menu** look: icon and text, icons only or text only. With **Size (zoom)** you make the whole app bigger or smaller (80% to 175%), like a computer's display scale. **High readability** is meant for people with low vision: maximum contrast, larger and bolder text, stronger borders and buttons, no animations; you can combine it with zoom. Your choices apply on all your devices.

**Avatar** — Also in **Profile**, press **Create your avatar**: choose hair, eyes, mouth, beard, glasses, clothes and colours while watching the preview, or press **Random**. Your character replaces your initials in chats, groups and attendance lists; **Remove avatar** brings the initials back.

**Password** — Change it on first login in **Profile → Change password** (at least 8 characters). If you forget it, ask an admin to reset it (Hiveloop doesn't send emails yet).

**Signing up** — If you don't have an account, press **Sign up** on the login page. Often an admin must approve the request before you can sign in.`,
      admin: `**Hiveloop** is our organisation's online space: calendar, group chats, documents and an AI assistant.

You are an **administrator (admin)**: besides everything a user can do, you manage **all** groups and **all** events and add and remove users.

**The menu** — left on a computer, bottom on a phone. Compared to users you also have :admin: **Manage**.

**Use it as an app on your phone** — iPhone: Share → *Add to Home Screen*. Android: menu :more: → *Install app*.

**Password** — Change it now in **Profile → Change password**. Users who forget their password will ask you: you can reset it from **Manage**.`,
      superadmin: `**Hiveloop** is our organisation's online space: calendar, group chats, documents and an AI assistant.

You are the **super admin**: full control. You can do everything an admin does and also **promote or demote admins** and **manage admin accounts**. There is only one super admin and nobody can change or delete your account.

**The menu** — left on a computer, bottom on a phone. You also have :admin: **Manage**.

**Security** — Use a long, unique password and never share it. When an admin leaves, remove their role or delete the account from **Manage**.`,
    })],
  },
  {
    id: "calendar", icon: "calendar", roles: ["user", "admin", "superadmin"],
    it: ["Calendario", (r) => `**Vedere le attività**
- **Mese**: la griglia del mese. Sul telefono i puntini colorati indicano che quel giorno ci sono eventi: tocca il giorno per vederli sotto.
- **Agenda**: l'elenco giorno per giorno.
- Frecce :left: :right: per cambiare mese, **Oggi** per tornare a oggi.
- Il colore di ogni evento è quello del suo gruppo.

**Filtri** (sotto i pulsanti)
- **Tutti**: tutto il calendario dell'organizzazione.
- **I miei gruppi**: solo gli eventi dei tuoi gruppi più quelli generali.
- **Nome di un gruppo**: solo quel gruppo.

**Aprire un evento** — Toccalo: vedi orario, luogo, gruppo e il **programma dettagliato**. In alto c'è **chi l'ha creato**: se hai un dubbio premi :chat: **Scrivi** e gli mandi un messaggio privato, con il nome dell'evento già scritto.

**Aggiungere un evento** — Premi **+ Nuovo evento** (su computer puoi anche cliccare due volte su un giorno). Compila titolo, inizio, fine, gruppo e, se vuoi, luogo e programma. Nel **luogo** scrivi nome e paese (es. *Municipio, Via Roma 1, Bologna*) oppure incolla un link di Google Maps: nell'evento diventa **Apri la mappa**. Il **programma** si scrive come i documenti: premi **+** accanto a una riga per titoli ed elenchi e seleziona il testo per grassetto, corsivo e link.

${r === "user"
  ? `**Cosa puoi modificare** — Puoi aggiungere, modificare ed eliminare **solo gli eventi dei gruppi di cui fai parte**. Gli eventi degli altri gruppi e quelli *Generali* li puoi solo guardare. Se non vedi il pulsante **+ Nuovo evento**, entra prima in un gruppo (vedi *Gruppi*).`
  : `**Cosa puoi modificare** — Come ${r === "admin" ? "admin" : "super admin"} puoi aggiungere, modificare ed eliminare gli eventi di **tutti i gruppi** e anche gli eventi **Generali** (scegli *Generale* nel campo Gruppo): sono quelli che riguardano tutta l'organizzazione.`}

**Segnare la presenza a un evento** — Apri l'evento e premi **Segna presenza**. Scegli **Tutto l'evento** (predefinito) oppure una **Fascia oraria** (es. dalle 10 alle 12 al banchetto) e, se vuoi, una nota (*"porto le sedie"*). Nel calendario gli eventi dove hai segnato la presenza sono evidenziati con :check: e mostrano il tuo orario; il numero indica quante persone ci saranno. Nell'evento, sotto **Presenze**, vedi chi c'è e in che orari. Puoi modificare o togliere la tua presenza quando vuoi. Il filtro **Le mie presenze** mostra solo i tuoi impegni.${r === "user" ? "" : " Chi può modificare l'evento può anche togliere la presenza di qualcuno."}

**Volontariə e turni** — Quando crei o modifichi un evento, nella parte **Volontariə** premi **+ Aggiungi turno** e indica orario e quante persone servono (es. *banchetto di Natale, 10–13, 2 volontariə*; puoi aggiungere un compito, come *cassa*). Nell'evento ogni turno mostra a che punto è (**1/2**, in rosso se mancano persone) e chi si è segnato: premi **Mi segno** per dare una mano, anche se non fai parte di quel gruppo, e **Tolgo** se non puoi più. Nel calendario il numero rosso indica quante persone mancano. Se un turno resta scoperto, i membri del gruppo (per gli eventi generali, tutti) ricevono un avviso **3 giorni, 1 giorno e 3 ore prima**, nella chat e come notifica; chi si è già segnatə non viene disturbatə.

**Eventi privati del gruppo** — Per riunioni e organizzazione interna spunta :lock: **Privato: solo il gruppo** nell'evento: lo vedono solo i membri del gruppo${r === "user" ? " e gli amministratori" : " (e voi amministratori)"}, con un lucchetto nel calendario. Un evento privato non si può condividere con un link pubblico.

**Condividere con chi non ha Hiveloop** — Apri un evento e premi :share: **Condividi**: ottieni un **link pubblico** da mandare (pulsanti *Copia link* e *WhatsApp*). Chi lo apre vede data, orario, luogo e programma e può aggiungerlo al proprio calendario, senza account; i nomi dei partecipanti non compaiono. Per mandare **tutto il programma di una giornata**, tocca il giorno e premi :share: accanto alla data: se hai scelto il filtro di un gruppo, il link mostra solo gli eventi di quel gruppo. Gli eventi aggiunti dopo compaiono da soli. Puoi togliere l'accesso in ogni momento con **Disattiva link**. Puoi anche chiederlo all'assistente: *"Dammi il link della giornata di sabato del gruppo Escursioni da mandare a chi vuole partecipare"*. Può condividere chi può modificare l'evento (membri del gruppo e amministratori); la giornata con tutti i gruppi solo gli amministratori.

**Messaggi rimasti** — Se c'è un limite giornaliero, in alto a sinistra nella pagina Assistente (e accanto al nome nelle chat di gruppo) vedi quanti messaggi all'assistente ti restano oggi, per esempio 7/10.

**Con l'assistente** — Puoi anche chiedere all'assistente: *"Segna la mia presenza al banchetto di sabato"* (ti chiederà a che ora ci sarai) oppure *"Aggiungi la riunione di sabato alle 10 per il gruppo X con questo programma…"*. Gli eventi creati così hanno l'etichetta **AI**.`],
    en: ["Calendar", (r) => `**Viewing activities**
- **Month**: the month grid. On a phone, coloured dots mean there are events that day: tap the day to see them below.
- **Agenda**: a day-by-day list.
- Arrows :left: :right: change month, **Today** jumps back.
- Each event has its group's colour.

**Filters** (below the buttons)
- **All**: the whole organisation's calendar.
- **My groups**: only your groups' events plus general ones.
- **A group name**: only that group.

**Opening an event** — Tap it: time, place, group and the **detailed program**. At the top you see **who created it**: if you have a question press :chat: **Message** to write to them privately, with the event's name already filled in.

**Adding an event** — Press **+ New event** (on a computer you can also click a day twice). Fill in title, start, end, group and optionally place and program. For the **place** write the name and town (e.g. *Town Hall, Main Street 1, Springfield*) or paste a Google Maps link: the event shows **Open map**. The **program** is written like documents: press **+** next to a line for headings and lists, and select text for bold, italic and links.

${r === "user"
  ? `**What you can change** — You can add, edit and delete **only events of groups you belong to**. Other groups' events and *General* events are read-only for you. If you don't see **+ New event**, join a group first (see *Groups*).`
  : `**What you can change** — As ${r === "admin" ? "an admin" : "super admin"} you can add, edit and delete events of **every group** and **General** events (choose *General* in the Group field): those concern the whole organisation.`}

**Marking attendance** — Open the event and press **Mark attendance**. Choose **Whole event** (default) or a **Time slot** (e.g. 10–12 at the stand) and optionally a note (*"bringing chairs"*). In the calendar, events where you marked attendance are highlighted with :check: and show your time; the number shows how many people will be there. Inside the event, under **Attendance**, you see who's coming and when. You can change or remove your attendance anytime. The **My attendance** filter shows only your commitments.${r === "user" ? "" : " Whoever can edit the event can also remove someone's attendance."}

**Volunteers and shifts** — When you create or edit an event, in **Volunteers** press **+ Add shift** and set the time and how many people are needed (e.g. *Christmas stall, 10–13, 2 volunteers*; you can add a task, like *till*). Inside the event each shift shows how it's going (**1/2**, red while people are missing) and who signed up: press **Sign me up** to help, even if you're not in that group, and **Remove me** if you can't make it any more. In the calendar the red number shows how many people are missing. If a shift is still short, the group's members (everyone for general events) are reminded **3 days, 1 day and 3 hours before**, in the chat and as a notification; whoever already signed up is not bothered.

**Private group events** — For meetings and internal organisation tick :lock: **Private: group only** in the event: only the group's members${r === "user" ? " and admins" : " (and you admins)"} see it, with a padlock in the calendar. A private event can't be shared with a public link.

**Sharing with people who don't use Hiveloop** — Open an event and press :share: **Share**: you get a **public link** to send (*Copy link* and *WhatsApp* buttons). Whoever opens it sees date, time, place and program and can add it to their calendar, without an account; participants' names are not shown. To send **a whole day's program**, tap the day and press :share: next to the date: if a group filter is selected, the link shows only that group's events. Events added later appear automatically. You can remove access anytime with **Turn off link**. You can also ask the assistant: *"Give me the link to Saturday's program for the Hiking group to send to people who want to join"*. Anyone who can edit the event can share it (group members and admins); a day with all groups only admins.

**Messages left** — If there is a daily limit, at the top left of the Assistant page (and next to the name in group chats) you see how many assistant messages you have left today, e.g. 7/10.

**With the assistant** — You can also ask: *"Mark me present at Saturday's stand"* (it will ask what time you'll be there) or *"Add Saturday's 10am meeting for group X with this program…"*. Events created this way are tagged **AI**.`],
  },
  {
    id: "chat", icon: "chat", roles: ["user", "admin", "superadmin"],
    it: ["Chat e menzioni @", (r) => `**Le chat**
- **Generale**: la chat di tutta l'organizzazione.
- Una chat per **ogni gruppo**: la vedono i membri del gruppo${r === "user" ? " (e gli amministratori)" : ". Tu, come amministratore, vedi e puoi scrivere in tutte le chat dei gruppi"}.
- Il numero accanto a **Chat** nel menu indica i messaggi non letti. Se è **arancione con @**, qualcuno ti ha menzionato.

**Scrivere** — Scrivi in basso e premi Invio (su computer) o la freccia. Su computer, *Maiusc + Invio* va a capo.

**Menzionare qualcuno con @**
- Scrivi **@** e compare l'elenco delle persone: scegli con un tocco (o frecce + Invio).
- La persona menzionata riceve un avviso e vede il messaggio evidenziato.
- **@tutti** avvisa tutte le persone della chat. Usalo con moderazione

**Chiedere all'AI in chat con @ai**
- Scrivi per esempio: *"@ai cosa c'è in programma questa settimana per il gruppo?"* oppure *"@ai riassumi il documento che ha mandato Marco"*.
- La risposta di **Hiveloop AI** compare nella chat e la leggono tutti.
- In chat l'AI legge **solo i documenti condivisi in quella chat** (mai quelli privati), può consultare e organizzare il calendario con i permessi di chi l'ha chiamata, e i file che crea vengono condivisi nella chat.

**Rispondere a un messaggio** — Sul telefono trascina il messaggio verso **sinistra**; su computer passa sopra il messaggio e premi :reply:. Sopra la casella compare il messaggio a cui rispondi (la :x: annulla). Nella chat la risposta mostra il messaggio originale: toccalo per ritrovarlo. Chi ha scritto l'originale riceve un avviso. **Se rispondi a un messaggio dell'assistente non serve scrivere @ai**: l'assistente continua la conversazione da dove era rimasto.

**Allegare un documento** — Tocca la :paperclip: graffetta e scegli uno dei tuoi documenti: viene **condiviso automaticamente** con quella chat. L'icona :docs: in alto mostra tutti i documenti condivisi nella chat.

**Eliminare un messaggio** — Passa sopra il messaggio (sul telefono toccalo) e premi il cestino. ${r === "user" ? "Puoi eliminare solo i tuoi messaggi." : "Come amministratore puoi eliminare anche i messaggi degli altri, se sono inappropriati."}

**Messaggi privati** — In fondo all'elenco delle chat premi :edit: **Nuovo messaggio** e scegli la persona; puoi anche toccare un membro nella pagina di un gruppo, o premere **Scrivi** in un evento per scrivere a chi l'ha creato. La conversazione la leggete **solo voi due**${r === "user" ? "" : " (neanche gli amministratori la vedono)"}. Nei messaggi privati non si allegano documenti e non c'è l'assistente.

**Notifiche** — Da **Profilo → Notifiche** premi **Attiva** (o la campanella :bell: nella pagina Chat): ricevi un avviso anche ad app chiusa per **messaggi privati**, **menzioni**, risposte ai tuoi messaggi e **turni di volontariato scoperti**. Va attivato su ogni dispositivo; **Prova** manda una notifica di prova. Su iPhone prima aggiungi Hiveloop alla schermata Home e aprilo da lì. Nell'app Android le notifiche arrivano dall'app (controlla ogni 15 minuti circa).`],
    en: ["Chat and @mentions", (r) => `**The chats**
- **General**: the whole organisation's chat.
- One chat for **each group**: its members can see it${r === "user" ? " (and admins)" : ". As an admin you can read and write in every group chat"}.
- The number next to **Chat** in the menu shows unread messages. **Orange with @** means someone mentioned you.

**Writing** — Type at the bottom and press Enter (computer) or the arrow. On a computer, *Shift + Enter* adds a new line.

**Mentioning someone with @**
- Type **@** and a list of people appears: pick one with a tap (or arrows + Enter).
- The person gets a notice and sees the message highlighted.
- **@all** (or **@tutti**) notifies everyone in the chat. Use it sparingly

**Asking the AI in a chat with @ai**
- For example: *"@ai what's planned for the group this week?"* or *"@ai summarise the document Marco sent"*.
- **Hiveloop AI** answers in the chat, for everyone to read.
- In chats the AI reads **only documents shared in that chat** (never private ones), can check and organise the calendar with the permissions of whoever called it, and files it creates are shared in the chat.

**Replying to a message** — On a phone drag the message to the **left**; on a computer hover over it and press :reply:. The message you're answering appears above the text box (:x: cancels). In the chat the reply shows the original message: tap it to jump there. Its author gets a notification. **When you reply to an assistant message you don't need to write @ai**: the assistant carries on the conversation.

**Attaching a document** — Tap the :paperclip: paperclip and pick one of your documents: it gets **shared automatically** with that chat. The :docs: icon at the top lists all documents shared in the chat.

**Deleting a message** — Hover the message (tap it on a phone) and press the bin. ${r === "user" ? "You can delete only your own messages." : "As an admin you can also delete other people's messages if they're inappropriate."}

**Private messages** — At the bottom of the chat list press :edit: **New message** and choose the person; you can also tap a member on a group's page, or press **Message** in an event to write to whoever created it. Only **the two of you** can read the conversation${r === "user" ? "" : " (admins can't see it either)"}. Private messages have no attachments and no assistant.

**Notifications** — In **Profile → Notifications** press **Turn on** (or the :bell: bell on the Chat page): you get notified even when the app is closed for **private messages**, **mentions**, replies to your messages and **volunteer shifts still short**. Turn it on on each device; **Test** sends a test notification. On iPhone add Hiveloop to the Home Screen first and open it from there. In the Android app notifications come from the app (it checks about every 15 minutes).`],
  },
  {
    id: "groups", icon: "groups", roles: ["user", "admin", "superadmin"],
    it: ["Gruppi", (r) => `I gruppi sono le squadre dell'organizzazione (es. *Logistica*, *Comunicazione*). Ogni gruppo ha un colore, una chat e i suoi eventi in calendario.

**Entrare e uscire** — Nella pagina **Gruppi** premi **Entra** sul gruppo che ti interessa. Per uscire, premi **Iscritto** (diventa *Esci*).

**La pagina del gruppo** — Tocca la scheda di un gruppo per aprirne la panoramica: descrizione, capogruppo, **prossimi eventi** (toccane uno per aprirlo nel calendario) e i pulsanti Chat e Calendario. Se sei iscritto vedi anche i **membri** e i **documenti** condivisi col gruppo; in fondo c'è **Esci dal gruppo**.${r === "user" ? " Se sei capogruppo, da qui gestisci membri e documenti e con :edit: **Modifica** cambi nome, descrizione e **immagine del gruppo**." : " Come amministratore da qui modifichi il gruppo (anche colore e **immagine**) e gestisci membri e documenti."}

**Perché entrare in un gruppo** — Vedi la sua chat, puoi organizzare i suoi eventi e ricevere i documenti condivisi con il gruppo.

Dalla scheda di un gruppo, le icone :calendar: e :chat: portano al suo calendario e alla sua chat.
${r === "user" ? "" : `
**Gestire i gruppi (amministratori)**
- **+ Nuovo gruppo**: nome, descrizione e colore.
- :edit: **Modifica**: cambia nome, descrizione, colore o elimina il gruppo. *Attenzione: eliminando un gruppo si cancellano anche i suoi eventi e la sua chat.*
- :groups: **Membri**: aggiungi o togli persone dal gruppo.
- In :admin: **Gestione**, scheda **Chat generale e assistente**: colore e foto della chat Generale e avatar dell'assistente.${r === "superadmin" ? "\n- Solo super admin, scheda **Documenti e limiti**: accendere o spegnere la creazione di documenti e il caricamento di file, lo spazio per utente e quanti messaggi al giorno ognuno (e tutti insieme) può mandare all'assistente; in **Limiti per gruppo** scegli se dare un limite a ogni chat di gruppo (in automatico, dividendo il limite per tutti in base ai membri, oppure a mano con valori suggeriti se i numeri non tornano); nella scheda **Assistente AI** puoi anche limitarlo a certi orari e giorni, e in **Modello AI** scegli il servizio (Ollama o un'API compatibile OpenAI), l'indirizzo, il modello e la chiave, con il pulsante **Prova** per controllare prima di salvare; da **Modifica** sulla persona si cambiano spazio e messaggi solo per lei. Nella lista utenti vedi spazio usato e messaggi di oggi." : ""}`}

**Capogruppo** — Ogni gruppo può avere un capogruppo, eletto dai membri. Finché non c'è, nella chat del gruppo compare il pulsante :crown: **Vota**: tocca la persona che vuoi; diventa capogruppo chi raggiunge la maggioranza dei membri (puoi cambiare voto finché nessuno è eletto). Il capogruppo può **aggiungere e togliere membri** del suo gruppo e **gestire i documenti condivisi col gruppo** (modificare i documenti di testo e toglierli dal gruppo; il file resta a chi l'ha caricato). Può lasciare il ruolo da **Membri**; se esce dal gruppo si vota di nuovo.${r === "user" ? "" : " Gli amministratori possono anche nominare o togliere un capogruppo da **Membri**."}`],
    en: ["Groups", (r) => `Groups are the organisation's teams (e.g. *Logistics*, *Communication*). Each group has a colour, a chat and its own calendar events.

**Joining and leaving** — On the **Groups** page press **Join** on a group. To leave, press **Joined** (it turns into *Leave*).

**The group page** — Tap a group's card to open its overview: description, leader, **upcoming events** (tap one to open it in the calendar) and the Chat and Calendar buttons. If you're a member you also see the **members** and the **documents** shared with the group; at the bottom there's **Leave group**.${r === "user" ? " If you're the leader, you manage members and documents from here and with :edit: **Edit** you change name, description and the **group image**." : " As an admin you edit the group (colour and **image** too) and manage members and documents from here."}

**Why join a group** — You see its chat, you can organise its events and receive documents shared with the group.

On a group card, the :calendar: and :chat: icons open its calendar and its chat.
${r === "user" ? "" : `
**Managing groups (admins)**
- **+ New group**: name, description and colour.
- :edit: **Edit**: change name, description, colour or delete the group. *Careful: deleting a group also deletes its events and its chat.*
- :groups: **Members**: add or remove people.
- In :admin: **Manage**, card **General chat and assistant**: colour and photo of the General chat and the assistant's avatar.${r === "superadmin" ? "\n- Super admin only, card **Documents and limits**: turn document creation and file uploads on or off, set space per user and how many assistant messages each person (and everyone together) can send per day; in **Limits per group** choose whether each group chat gets a limit (automatically, sharing the limit for everyone by members, or by hand with suggested values when the numbers don't add up); in the **AI assistant** card you can also limit it to certain hours and days, and in **AI model** choose the service (Ollama or an OpenAI-compatible API), address, model and key, with **Test** to check before saving; **Edit** on a person changes space and messages just for them. The users list shows space used and today's messages." : ""}`}

**Group leader** — Each group can have a leader, elected by its members. While there is none, the group chat shows a :crown: **Vote** button: tap the person you want; whoever gets a majority of the members becomes leader (you can change your vote until someone is elected). The leader can **add and remove members** of their group and **manage documents shared with the group** (edit text documents and remove them from the group; the file stays with whoever uploaded it). They can step down from **Members**; if they leave the group a new vote starts.${r === "user" ? "" : " Admins can also appoint or remove a leader from **Members**."}`],
  },
  {
    id: "docs", icon: "docs", roles: ["user", "admin", "superadmin"],
    it: ["Documenti", (r) => `Ognuno ha il **proprio spazio documenti** (fino a 25 MB per file, 500 MB in totale).

**Caricare** — Premi **Carica** o trascina i file nel riquadro tratteggiato.

**Privati per impostazione** — Un file appena caricato lo vedi **solo tu** (e il tuo assistente). L'etichetta :lock: *Privato* lo ricorda.

**Condividere** — Premi l'icona **Condividi** accanto al file e scegli:
- **Tutti**: lo vede tutta l'organizzazione.
- **uno o più gruppi**: lo vedono i membri di quei gruppi${r === "user" ? " (puoi scegliere solo i gruppi di cui fai parte)" : ""}.
Togli tutte le spunte per renderlo di nuovo privato. Anche se condiviso, **solo tu** puoi eliminarlo o cambiare con chi è condiviso.

**Modificare i documenti (.md)** — I documenti di testo creati dall'assistente o con **Nuovo documento** si modificano come in un normale programma di scrittura, senza vedere codice: premi :edit: **Modifica**. Premi **+** (o Tab su una riga vuota) per aggiungere titoli, elenchi, liste di controllo, tabelle, citazioni o separatori; seleziona il testo per **grassetto**, *corsivo* e link; con :grip: accanto a un blocco lo sposti, lo trasformi o lo elimini. Il salvataggio è automatico. ${r === "user" ? "Puoi modificare i tuoi documenti." : "Puoi modificare i tuoi documenti e, come amministratore, anche quelli .md condivisi con te."}

**Versioni precedenti (.md)** — Prima di ogni modifica Hiveloop conserva la versione precedente (le ultime 10). Chi può modificare il documento trova il pulsante :history: **Versioni precedenti** accanto a **Modifica**: tocca una versione per leggerla (in alto è segnata come *versione vecchia*) e premi **Ripristina** per riportarla. Niente va perso: il testo attuale finisce a sua volta tra le versioni precedenti. Puoi anche chiedere all'assistente *"Cosa è cambiato nel prezzario rispetto a ieri?"* oppure *"Recupera la parte sugli orari che è stata cancellata"*.

**Condivisi con me** — Nella seconda scheda trovi i file che altri hanno condiviso con te o con i tuoi gruppi.

**Anteprima e download** — Tocca il nome per vedere l'anteprima (PDF, immagini, testo, Markdown) o usa :download: per scaricare.

**Con l'assistente** — Puoi chiedere: *"Condividi il verbale con il gruppo Logistica"* oppure *"Leggi il PDF del regolamento e fammi un riassunto in un file .md"*. L'assistente crea solo file **Markdown (.md)**.`],
    en: ["Documents", (r) => `Everyone has their **own document space** (up to 25 MB per file, 500 MB total).

**Uploading** — Press **Upload** or drag files onto the dashed box.

**Private by default** — A newly uploaded file is visible **only to you** (and your assistant). The :lock: *Private* tag reminds you.

**Sharing** — Press the **Share** icon next to the file and choose:
- **Everyone**: the whole organisation sees it.
- **one or more groups**: members of those groups see it${r === "user" ? " (you can only pick groups you belong to)" : ""}.
Untick everything to make it private again. Even when shared, **only you** can delete it or change who it's shared with.

**Editing documents (.md)** — Text documents created by the assistant or with **New document** are edited like in a normal word processor, without seeing any code: press :edit: **Edit**. Press **+** (or Tab on an empty line) to add headings, lists, checklists, tables, quotes or dividers; select text for **bold**, *italic* and links; use :grip: next to a block to move, convert or delete it. Saving is automatic. ${r === "user" ? "You can edit your own documents." : "You can edit your own documents and, as an admin, also .md documents shared with you."}

**Previous versions (.md)** — Before every change Hiveloop keeps the previous version (the last 10). Whoever can edit the document finds the :history: **Previous versions** button next to **Edit**: tap a version to read it (it is marked as an *old version* at the top) and press **Restore** to bring it back. Nothing is lost: the current text is kept among the previous versions too. You can also ask the assistant *"What changed in the price list since yesterday?"* or *"Recover the part about opening hours that was deleted"*.

**Shared with me** — The second tab lists files others shared with you or your groups.

**Preview and download** — Tap the name for a preview (PDF, images, text, Markdown) or use :download: to download.

**With the assistant** — Ask: *"Share the minutes with the Logistics group"* or *"Read the rules PDF and summarise it in a .md file"*. The assistant only creates **Markdown (.md)** files.`],
  },
  {
    id: "ai", icon: "ai", roles: ["user", "admin", "superadmin"],
    it: ["Assistente AI", (r) => `L'assistente è un'intelligenza artificiale che lavora **con i tuoi stessi permessi**: non può fare nulla che tu non potresti fare.

**Cosa può fare**
- Rispondere a domande e aiutarti a scrivere.
- Consultare il calendario: *"Cosa c'è venerdì?"*
- Aggiungere, spostare o togliere eventi con un programma dettagliato: *"Organizza la festa del 12 dicembre per il gruppo X, dalle 16 alle 20, con scaletta"*. ${r === "user" ? "Solo per i gruppi di cui fai parte." : "Per qualsiasi gruppo o come evento generale."}
- Leggere i tuoi documenti e quelli condivisi con te (PDF, testo, Markdown…).
- Creare **file Markdown (.md)** nel tuo spazio (es. verbali, riassunti, checklist) e **modificare quelli esistenti** che puoi modificare anche tu: *"Aggiungi al verbale i punti decisi oggi"*, *"Correggi gli orari nella scaletta"*.
- **Condividere** i tuoi documenti con un gruppo o con tutti, se glielo chiedi.
- **Cercare su internet** (con DuckDuckGo) e leggere le pagine trovate: *"Cerca i fornitori di gazebo in zona e confronta i prezzi"*. Nella risposta trovi i link alle fonti: controllali sempre.

**Cosa non può fare** — Creare altri tipi di file, eliminare documenti, vedere i documenti privati degli altri.

**Conversazioni** — Ogni chat con l'assistente viene salvata a sinistra (sul telefono: icona :chat:). **+ Nuova chat** per iniziare un nuovo argomento.

**Nelle chat di gruppo** — Scrivi **@ai** in un messaggio per far rispondere l'assistente davanti a tutto il gruppo (vedi *Chat*).

**Consiglio** — Più sei preciso (data, ora, gruppo), migliore è il risultato. Controlla sempre gli eventi creati: l'AI può sbagliare.`],
    en: ["AI assistant", (r) => `The assistant is an AI that works **with your own permissions**: it can't do anything you couldn't do.

**What it can do**
- Answer questions and help you write.
- Check the calendar: *"What's on Friday?"*
- Add, move or remove events with a detailed program: *"Plan the party on 12 December for group X, 4–8pm, with a running order"*. ${r === "user" ? "Only for groups you belong to." : "For any group or as a general event."}
- Read your documents and those shared with you (PDF, text, Markdown…).
- Create **Markdown (.md) files** in your space (minutes, summaries, checklists) and **change existing ones** you are allowed to edit: *"Add today's decisions to the minutes"*, *"Fix the times in the running order"*.
- **Share** your documents with a group or everyone, when you ask.
- **Search the internet** (with DuckDuckGo) and read the pages it finds: *"Find gazebo suppliers nearby and compare prices"*. Answers include links to the sources: always check them.

**What it can't do** — Create other file types, delete documents, see other people's private documents.

**Conversations** — Each chat with the assistant is saved on the left (on a phone: :chat: icon). **+ New chat** starts a new topic.

**In group chats** — Write **@ai** in a message to have the assistant answer in front of the whole group (see *Chat*).

**Tip** — The more precise you are (date, time, group), the better. Always check created events: AI can make mistakes.`],
  },
  {
    id: "users", icon: "admin", roles: ["admin", "superadmin"],
    it: ["Gestione utenti", (r) => `Dalla pagina **Gestione** gestisci le persone che usano Hiveloop.

**Aggiungere una persona** — **+ Nuovo utente** → nome utente (3-32 caratteri, senza spazi, es. *mario.rossi*), nome visibile, password iniziale (almeno 8 caratteri)${r === "superadmin" ? ", **ruolo** (Utente o Admin)" : ""} e, se vuoi, i gruppi. Poi manda alla persona il link, il nome utente e la password, chiedendole di cambiarla al primo accesso.

**Richieste di registrazione** — Quando qualcuno si registra, in cima a **Gestione** compare *Richieste di registrazione* (e un numero sul menu). Premi **Approva** per attivare l'account oppure :x: per rifiutarlo. Approva solo persone che conosci.
${r === "superadmin" ? `
**Ricerca web dell'assistente (solo super admin)** — In Gestione puoi inserire o cambiare la chiave **Brave Search**: con la chiave l'assistente trova risultati affidabili; senza, usa motori gratuiti che spesso bloccano o danno risultati poco pertinenti. Il pulsante **Prova** verifica che funzioni.

**Modalità di registrazione (solo super admin)** — In cima a Gestione scegli: **Chiusa** (solo gli admin creano account), **Con approvazione** (consigliata) o **Aperta** (chiunque entra subito: attenzione, anche sconosciuti potrebbero usare l'assistente AI, che ha un costo).
` : ""}
**Reimpostare una password** — :edit: accanto alla persona → *Reimposta password*. La persona verrà disconnessa da tutti i dispositivi.

**Rimuovere una persona** — :trash: accanto al nome. Vengono eliminati anche **tutti i suoi documenti**; i suoi eventi restano.

${r === "superadmin"
  ? `**Ruoli (solo super admin)** — Con :edit: puoi trasformare un utente in **Admin** o riportare un admin a **Utente**. Puoi anche modificare o eliminare gli account degli admin. Il tuo account di super admin non può essere modificato né eliminato da nessuno.`
  : `**Limiti** — Come admin puoi gestire solo gli **utenti normali**. Gli altri admin e il super admin li può gestire solo il super admin. Non puoi creare altri admin.`}`],
    en: ["Managing users", (r) => `On the **Manage** page you manage the people using Hiveloop.

**Adding someone** — **+ New user** → username (3-32 characters, no spaces, e.g. *mario.rossi*), display name, initial password (at least 8 characters)${r === "superadmin" ? ", **role** (User or Admin)" : ""} and optionally groups. Then send them the link, username and password and ask them to change it at first login.

**Sign-up requests** — When someone signs up, *Sign-up requests* appears at the top of **Manage** (with a number on the menu). Press **Approve** to activate the account or :x: to reject it. Only approve people you know.
${r === "superadmin" ? `
**Assistant web search (super admin only)** — In Manage you can enter or change the **Brave Search** key: with it the assistant gets reliable results; without it, it uses free engines that often block or return poor results. **Test** checks that it works.

**Sign-up mode (super admin only)** — At the top of Manage choose: **Closed** (only admins create accounts), **With approval** (recommended) or **Open** (anyone gets in immediately: careful, strangers could use the AI assistant, which costs money).
` : ""}
**Resetting a password** — :edit: next to the person → *Reset password*. They'll be signed out everywhere.

**Removing someone** — :trash: next to the name. **All their documents** are deleted too; their events stay.

${r === "superadmin"
  ? `**Roles (super admin only)** — With :edit: you can turn a user into an **Admin** or back to **User**. You can also edit or delete admin accounts. Your super admin account can't be changed or deleted by anyone.`
  : `**Limits** — As an admin you can manage **regular users** only. Other admins and the super admin are managed by the super admin. You can't create admins.`}`],
  },
  {
    id: "roles", icon: "admin", roles: ["user", "admin", "superadmin"],
    it: ["Chi può fare cosa", `| | Utente | Admin | Super admin |
|---|:-:|:-:|:-:|
| Vedere tutto il calendario | :check: | :check: | :check: |
| Eventi dei propri gruppi | :check: | :check: | :check: |
| Eventi di tutti i gruppi e generali | | :check: | :check: |
| Entrare/uscire dai gruppi | :check: | :check: | :check: |
| Creare/modificare gruppi e membri | | :check: | :check: |
| Chat dei propri gruppi e generale | :check: | :check: | :check: |
| Leggere tutte le chat dei gruppi | | :check: | :check: |
| Documenti personali e condivisione | :check: | :check: | :check: |
| Assistente AI | :check: | :check: | :check: |
| Aggiungere/rimuovere utenti | | :check: | :check: |
| Gestire gli admin e i ruoli | | | :check: |`],
    en: ["Who can do what", `| | User | Admin | Super admin |
|---|:-:|:-:|:-:|
| See the whole calendar | :check: | :check: | :check: |
| Events of own groups | :check: | :check: | :check: |
| Events of all groups and general | | :check: | :check: |
| Join/leave groups | :check: | :check: | :check: |
| Create/edit groups and members | | :check: | :check: |
| Own groups' chats and general chat | :check: | :check: | :check: |
| Read all group chats | | :check: | :check: |
| Personal documents and sharing | :check: | :check: | :check: |
| AI assistant | :check: | :check: | :check: |
| Add/remove users | | :check: | :check: |
| Manage admins and roles | | | :check: |`],
  },
  {
    id: "faq", icon: "key", roles: ["user", "admin", "superadmin"],
    it: ["Problemi frequenti", (r) => `**Ho dimenticato la password** — ${r === "user" ? "Chiedi a un amministratore di reimpostarla." : r === "admin" ? "Chiedi al super admin o a un altro admin." : "Se hai ancora accesso a un altro dispositivo, cambiala da Profilo; altrimenti serve un intervento tecnico sul server."}

**Non vedo il pulsante + Nuovo evento** — ${r === "user" ? "Devi far parte di almeno un gruppo: entra in un gruppo dalla pagina Gruppi." : "Ricarica la pagina; come amministratore dovresti sempre vederlo."}

**Non vedo la chat di un gruppo** — ${r === "user" ? "Entra nel gruppo dalla pagina Gruppi." : "Controlla che il gruppo esista ancora nella pagina Gruppi."}

**L'assistente non risponde o dà errore** — Riprova tra poco: potrebbe essere occupato. Se succede spesso, avvisa ${r === "superadmin" ? "il tecnico che gestisce il server" : "il super admin"}.

**Non ricevo notifiche** — Le notifiche funzionano mentre Hiveloop è aperta (anche in un'altra scheda) e dopo aver premuto *Attiva notifiche* nella pagina Chat.

**Un file non si apre in anteprima** — Alcuni formati (es. Word, Excel) non hanno anteprima: scaricalo con :download:.`],
    en: ["Common problems", (r) => `**I forgot my password** — ${r === "user" ? "Ask an admin to reset it." : r === "admin" ? "Ask the super admin or another admin." : "If you're still signed in on another device, change it from Profile; otherwise it needs a technical fix on the server."}

**I don't see + New event** — ${r === "user" ? "You must belong to at least one group: join one from the Groups page." : "Reload the page; as an admin you should always see it."}

**I can't see a group's chat** — ${r === "user" ? "Join the group from the Groups page." : "Check the group still exists on the Groups page."}

**The assistant doesn't answer or shows an error** — Try again shortly: it may be busy. If it keeps happening, tell ${r === "superadmin" ? "whoever runs the server" : "the super admin"}.

**I don't get notifications** — They work while Hiveloop is open (even in another tab) and after pressing *Enable notifications* on the Chat page.

**A file has no preview** — Some formats (e.g. Word, Excel) can't be previewed: download it with :download:.`],
  },
];

export function helpSections(role, lang) {
  return SECTIONS.filter((s) => s.roles.includes(role)).map((s) => {
    const [title, body] = s[lang] || s.it;
    return { id: s.id, icon: s.icon, title, body: pick(role, body) };
  });
}
