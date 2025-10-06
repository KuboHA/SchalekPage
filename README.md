# SchalekPage

> **Status:** Early alpha. This repository is a work-in-progress prototype and does not represent the final product.

SchalekPage is a Flask-based web dashboard that integrates with the EduPage platform to provide students with a consolidated view of their school life. It surfaces notifications, timetable information, meal plans, substitution changes, and more—wrapped in a modern UI with rich attachment previews and threaded conversations.

## ✨ Features

- **EduPage authentication** with optional two-factor support.
- **Dashboard overview** of recent notifications, grouped threads, confirmation counts, and rich formatting.
- **Attachment handling** with automatic URL normalization, downloadable files, and inline image previews for `.jpg`/`.jpeg` assets.
- **Timetable viewer** with date navigation and automatic next-day rollover after 14:30.
- **Meals module** that lists ordered snacks, lunches, and afternoon snacks.
- **Substitution changes** filtered to the logged-in student’s class and normalized for friendly display.
- **Grades overview** (term-based) with newest grades first.

## 📁 Project Structure

```
SchalekPage/
├── app.py                # Flask application with all routes and data processing
├── index.html            # Landing page
├── templates/            # Jinja2 templates for dashboard, timetable, grades, etc.
└── static/
    └── css/              # Tailwind-inspired styling
```

## ✅ Requirements

- Python 3.10+
- Pip (Python package manager)
- EduPage credentials for a valid student account
- (Optional) Redis or database is **not** required—sessions are cookie-based

> **Note:** The repository does not ship with a pinned `requirements.txt`. The core dependencies are:
>
> - `flask`
> - `edupage_api`
>
> Install them manually or create a `requirements.txt` based on your environment.

## 🛠️ Setup & Installation

1. **Clone the repository**
   ```powershell
   git clone https://github.com/KuboHA/SchalekPage.git
   cd SchalekPage
   ```

2. **Create and activate a virtual environment (recommended)**
   ```powershell
   python -m venv .venv
   .venv\Scripts\Activate
   ```

3. **Install dependencies**
   ```powershell
   pip install flask edupage_api
   ```
   If you assemble your own `requirements.txt`, use `pip install -r requirements.txt` instead.

4. **Set environment variables (optional but recommended)**
   - `FLASK_SECRET_KEY` — override the default development secret key.
   - `FLASK_ENV` — set to `development` for auto-reload.

## ▶️ Running the App

```powershell
python app.py
```

The server runs on `http://127.0.0.1:5000/` by default. Open that URL in your browser to log in with your EduPage credentials.

### Two-Factor Authentication
If your EduPage account uses 2FA, you’ll be redirected to the `/two_factor` route to supply the verification code. Sessions persist the `PHPSESSID` cookie so you don’t have to re-enter credentials during the session.

## 💡 Usage Tips

- **Notifications:** Confirmations (“likes”) are counted heuristically. Attachments are grouped under the parent notification, and replies are nested based on explicit parent IDs or ID prefixes.
- **Attachments:**
  - Relative file paths are automatically prefixed with `https://{subdomain}.edupage.org`.
  - `.jpg`/`.jpeg` files open a rich preview with zoom, pan, and download controls.
  - Other files download directly when clicked.
- **Timetable:** After 14:30, the dashboard automatically shows the next day’s timetable to help students plan ahead.
- **Substitutions:** Only substitutions relevant to the logged-in student’s class are shown. Actions are normalized to lowercase strings (`change`, `add`, `remove`).

## 🔐 Security Notes

- The application currently stores the EduPage password in the session for re-authentication. For production usage, consider encrypting session data or switching to server-side session storage.
- Replace the hard-coded `app.secret_key` with an environment variable before deploying publicly.

## 🧪 Testing

There is no automated test suite yet. Suggested next steps:

- Add unit tests for notification enrichment (confirmations, attachments, threading).
- Mock EduPage API responses to ensure routes render without live credentials.
- Introduce integration tests using Flask’s test client.

## 🛣️ Roadmap & Ideas

- Persist tokens securely and avoid storing plain-text passwords in the session.
- Add pagination or lazy loading for long notification lists.
- Support additional attachment preview types (PDF, PNG, DOCX).
- Externalize configuration into `.env` files with `python-dotenv`.
- Implement role-based access (e.g., teachers vs. students).

## 📄 License

This project is distributed under the terms of the [MIT License](LICENSE).

---
Feel free to open issues or pull requests if you find bugs or want to contribute enhancements!
