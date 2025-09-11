from flask import Flask, render_template, request, redirect, url_for, session, jsonify
import json
from edupage_api import Edupage
from datetime import datetime, date, timedelta
from typing import Optional, Union
from enum import Enum

app = Flask(__name__)
app.secret_key = '738-531-827'

class EduStudent:
    def __init__(self, person_id: int, name: str, gender: str, in_school_since: Optional[datetime], class_id: int, number_in_class: int):
        self.person_id = person_id
        self.name = name
        self.gender = gender
        self.in_school_since = in_school_since
        self.class_id = class_id
        self.number_in_class = number_in_class

def time_since_posted(timestamp):
    now = datetime.now()
    diff = now - timestamp

    if diff.days > 0:
        return f"{diff.days} days ago"
    elif diff.seconds // 3600 > 0:
        return f"{diff.seconds // 3600} hours ago"
    elif diff.seconds // 60 > 0:
        return f"{diff.seconds // 60} minutes ago"
    else:
        return "Just now"

@app.route('/')
def index():
    return render_template('login.html')

@app.route('/login', methods=['POST'])
def login():
    username = request.form['username']
    password = request.form['password']
    subdomain = request.form['subdomain']

    edupage = Edupage()
    try:
        two_factor_login = edupage.login(username, password, subdomain)

        student_info = None
        try:
            students = edupage.get_students()
            for student in students:
                if username.lower() in student.name.lower().replace(' ', ''):
                    student_info = student
                    break
        except Exception as e:
            print(f"Error getting student info: {e}")

        if two_factor_login:
            session['subdomain'] = subdomain
            session['username'] = student_info.name if student_info else username
            session['student_id'] = student_info.person_id if student_info else None
            session['password'] = password  # Store password in session
            session['session_id'] = edupage.session.cookies.get('PHPSESSID')
            return redirect(url_for('two_factor'))
        else:
            session['subdomain'] = subdomain
            session['username'] = student_info.name if student_info else username
            session['student_id'] = student_info.person_id if student_info else None
            session['password'] = password
            session['session_id'] = edupage.session.cookies.get('PHPSESSID')
            return redirect(url_for('dashboard'))
    except Exception as e:
        return render_template('login.html', error=str(e))

@app.route('/two_factor', methods=['GET', 'POST'])
def two_factor():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    if request.method == 'POST':
        code = request.form['code']
        edupage = Edupage()
        try:
            edupage.login(session['username'], session['password'], session['subdomain'])
            edupage.session.cookies.set('PHPSESSID', session['session_id'])

            two_factor_login = edupage.login(session['username'], session['password'], session['subdomain'])
            two_factor_login.finish_with_code(code)

            session['session_id'] = edupage.session.cookies.get('PHPSESSID')
            return redirect(url_for('dashboard'))
        except Exception as e:
            return render_template('two_factor.html', error=str(e))

    return render_template('two_factor.html')

@app.route('/dashboard')
def dashboard():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    students = edupage.get_students()
    if 'student_id' in session and session['student_id']:
        student_data = next((student for student in students if student.person_id == session['student_id']), None)
    else:
        student_data = next((student for student in students if student.name == session['username']), None)
    
    notifications = edupage.get_notifications()
    # Base URL for attachment prefixing
    base_url = f"https://{session.get('subdomain')}.edupage.org" if session.get('subdomain') else None
    
    today = date.today()
    
    def should_show_tomorrow():
        now = datetime.now()
        cutoff = now.replace(hour=14, minute=30, second=0, microsecond=0)
        return now >= cutoff

    if should_show_tomorrow():
        target_date = today + timedelta(days=1)
    else:
        target_date = today

    timetable = edupage.get_my_timetable(target_date)

    # --- Notifications enrichment (grouping, attachments, confirmations) ---
    def _extract_confirm_count(data):
        if data is None:
            return 0
        count = 0
        # Direct numeric
        if isinstance(data, (int, float)):
            return int(data)
        if isinstance(data, (list, tuple, set)):
            return len(data)
        if isinstance(data, dict):
            # direct keys
            key_candidates = [k for k in data.keys() if any(x in k.lower() for x in ['confirm', 'like', 'thumb'])]
            for k in key_candidates:
                sub = data.get(k)
                sub_count = _extract_confirm_count(sub)
                if sub_count > count:
                    count = sub_count
            # common pattern inside dict: count field + users
            if 'count' in data and isinstance(data.get('count'), (int, float)):
                count = max(count, int(data.get('count')))  # prefer explicit
            return count
        # Fallback
        return 0

    id_map = {}
    for n in notifications:
        n.formatted_timestamp = time_since_posted(n.timestamp)
        n.replies = []
        n.attachments = []
        n.confirmation_count = 0
        add_data = getattr(n, 'additional_data', None)
        # Parse additional data JSON string if needed
        if isinstance(add_data, str):
            try:
                add_data = json.loads(add_data)
            except Exception:
                add_data = None
        # Confirmation count (recursive heuristic)
        n.confirmation_count = _extract_confirm_count(add_data)
        # Attachment extraction
        if isinstance(add_data, dict):
            def _record_attachment(att_name, att_url):
                if not att_url:
                    return
                if base_url and not att_url.lower().startswith(('http://', 'https://')):
                    if not att_url.startswith('/'):
                        att_url_local = '/' + att_url
                    else:
                        att_url_local = att_url
                    att_url_full = base_url.rstrip('/') + att_url_local
                else:
                    att_url_full = att_url
                lower_name = (att_name or '').lower()
                is_image = lower_name.endswith('.jpg') or lower_name.endswith('.jpeg')
                # Avoid duplicates
                if all(a['url'] != att_url_full for a in n.attachments):
                    n.attachments.append({'name': att_name or 'attachment', 'url': att_url_full, 'is_image': is_image})

            attachment_keys = ['attachments','files','prilohy','docs']
            for akey in attachment_keys:
                if akey in add_data:
                    raw_list = add_data.get(akey)
                    # List/Tuple style
                    if isinstance(raw_list, (list, tuple)):
                        for item in raw_list:
                            att_name = 'attachment'
                            att_url = None
                            if isinstance(item, dict):
                                att_name = item.get('name') or item.get('filename') or item.get('title') or 'attachment'
                                att_url = item.get('url') or item.get('link') or item.get('downloadUrl') or item.get('href')
                            elif isinstance(item, str):
                                att_url = item
                                att_name = item.split('/')[-1]
                            _record_attachment(att_name, att_url)
                    # Dict style (mapping path->filename or meta objects)
                    elif isinstance(raw_list, dict):
                        for k2, v2 in raw_list.items():
                            if isinstance(v2, dict):
                                att_name = v2.get('name') or v2.get('filename') or v2.get('title') or v2.get('file') or 'attachment'
                                att_url = v2.get('url') or v2.get('link') or v2.get('downloadUrl') or v2.get('href') or k2
                            else:
                                att_name = str(v2) if isinstance(v2, str) else k2.split('/')[-1]
                                att_url = k2 if isinstance(k2, str) else None
                            _record_attachment(att_name, att_url)
                    break

            # Recursive scan for '/elearning/' style entries
            def _scan(obj):
                if isinstance(obj, dict):
                    for k, v in obj.items():
                        if isinstance(k, str):
                            if '/elearning/' in k or k.startswith('/elearning') or k.startswith('elearning/'):
                                # k is path; v maybe filename
                                filename = v.strip() if isinstance(v, str) and v.strip() else k.split('/')[-1]
                                _record_attachment(filename, k)
                            if isinstance(v, str) and ('/elearning/' in v or v.startswith('/elearning') or v.startswith('elearning/')):
                                # v is path; k might be filename
                                filename = k.split('/')[-1]
                                _record_attachment(filename, v)
                        _scan(v)
                elif isinstance(obj, (list, tuple)):
                    for item in obj:
                        _scan(item)
            _scan(add_data)
        event_id = getattr(n, 'event_id', None)
        if event_id is None:
            event_id = id(n)
        id_map[event_id] = n

    # Threading using explicit parent keys or ID prefix heuristic
    parent_keys = ['parent_id','parentId','reply_to','replyTo','parentTimelineId','timeline_parent_id']
    # Build string IDs for heuristic
    event_id_strings = {eid: str(eid) for eid in id_map.keys()}

    def _guess_parent_by_prefix(eid):
        s = event_id_strings.get(eid)
        if not s:
            return None
        # Try shorter prefixes (length 4 or 5) as parent candidates
        for plen in (4,5):
            if len(s) > plen:
                prefix = s[:plen]
                # exact parent id exists?
                try:
                    pid = int(prefix)
                except ValueError:
                    continue
                if pid in id_map and pid != eid:
                    return pid
        return None

    main_notifications = []
    for eid, n in list(id_map.items()):
        add_data = getattr(n, 'additional_data', None)
        if isinstance(add_data, str):
            try:
                add_data = json.loads(add_data)
            except Exception:
                add_data = None
        parent_id = None
        if isinstance(add_data, dict):
            for pk in parent_keys:
                if pk in add_data and add_data.get(pk):
                    parent_id = add_data.get(pk)
                    if isinstance(parent_id, str) and parent_id.isdigit():
                        parent_id = int(parent_id)
                    break
        if not parent_id:
            parent_id = _guess_parent_by_prefix(eid)
        if parent_id and parent_id in id_map and parent_id != eid:
            id_map[parent_id].replies.append(n)
        else:
            main_notifications.append(n)

    main_notifications.sort(key=lambda x: x.timestamp, reverse=True)
    for n in main_notifications:
        n.replies.sort(key=lambda x: x.timestamp)
    notifications = main_notifications
    
    now = datetime.now()
    cutoff_time = now.replace(hour=14, minute=30, second=0, microsecond=0)
    show_tomorrow = now >= cutoff_time
    
    today = date.today()
    target_date = today + timedelta(days=1) if show_tomorrow else today
    
    meals_data = edupage.get_meals(target_date)
    
    meals_list = []
    if meals_data:
        if meals_data.snack:
            meals_list.append({
                'type': 'Snack',
                'menus': meals_data.snack.menus,
                'ordered_meal': meals_data.snack.ordered_meal if meals_data.snack.ordered_meal else 'X'
            })
        if meals_data.lunch:
            meals_list.append({
                'type': 'Lunch',
                'menus': meals_data.lunch.menus,
                'ordered_meal': meals_data.lunch.ordered_meal if meals_data.lunch.ordered_meal else 'X'
            })
        if meals_data.afternoon_snack:
            meals_list.append({
                'type': 'Afternoon Snack',
                'menus': meals_data.afternoon_snack.menus,
                'ordered_meal': meals_data.afternoon_snack.ordered_meal if meals_data.afternoon_snack.ordered_meal else 'X'
            })

    return render_template('dashboard.html', 
                         student=student_data,
                         notifications=notifications,
                         timetable=timetable,
                         meals=meals_list,
                         show_tomorrow=show_tomorrow,
                         event_type_map=EVENT_TYPE_MAP,
                         event_type_icons=EVENT_TYPE_ICONS)
    # NOTE FOR TEMPLATE: Each attachment dict now contains keys: name, url, is_image (bool for .jpg/.jpeg)
"""NOTE: Removed local Action enum duplicate.
The edupage_api.substitution module already defines Action(str, Enum).
Keeping a duplicate here risks confusion when comparing enum instances in templates.
Templates compare against raw string values (e.g. "change"), so we'll normalize
the action to its primitive string value before rendering.
"""

@app.route('/substitutions/', defaults={'date_str': None})
@app.route('/substitutions/<date_str>')
def get_timetable_changes(date_str):
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    # Recreate the Edupage object
    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])
    
    try:
        if date_str:
            specific_date = datetime.strptime(date_str, '%Y-%m-%d').date()
        else:
            specific_date = date.today()
    except ValueError:
        specific_date = date.today()

    prev_date = specific_date - timedelta(days=1)
    next_date = specific_date + timedelta(days=1)

    students = edupage.get_students()
    if 'student_id' in session and session['student_id']:
        student_data = next((student for student in students if student.person_id == session['student_id']), None)
    else:
        student_data = next((student for student in students if student.name == session['username']), None)
    
    # Define class_id from student data
    class_id = None
    if student_data:
        # Check if student_data has class_id attribute directly
        if hasattr(student_data, 'class_id'):
            class_id = student_data.class_id
        # Try alternative ways to get class_id if the direct attribute isn't available
        elif hasattr(student_data, 'data') and isinstance(student_data.data, dict):
            class_id = student_data.data.get('triedaid')
        elif hasattr(student_data, '_class_id'):
            class_id = student_data._class_id
    
    # Retrieve substitution changes safely
    try:
        substitution_module = edupage.substitution()
        raw_changes = substitution_module.get_timetable_changes(specific_date) or []
    except Exception as e:
        print(f"Error fetching substitution data: {e}")
        raw_changes = []

    # Resolve student's class name (short preferred)
    class_name = None
    if class_id:
        try:
            for class_obj in edupage.get_classes():
                if getattr(class_obj, 'class_id', None) == class_id:
                    class_name = getattr(class_obj, 'short', None) or getattr(class_obj, 'name', None)
                    break
        except Exception as e:
            print(f"Error resolving class name: {e}")

    # Normalize changes for template (avoid direct dependency on library internals)
    normalized_changes = []
    for ch in raw_changes:
        # Filter by student class if we have one
        if class_name and getattr(ch, 'change_class', None) != class_name:
            continue

        # Extract/normalize lesson number (may be int or tuple range)
        lesson_n = getattr(ch, 'lesson_n', None)
        if isinstance(lesson_n, tuple):
            lesson_display = f"{lesson_n[0]}-{lesson_n[1]}"
        else:
            lesson_display = lesson_n

        # Normalize action (enum or primitive)
        action_val = getattr(ch, 'action', None)
        # Action may be enum subclass of str or plain str
        if hasattr(action_val, 'value'):
            action_val = action_val.value
        # Some libraries might return tuple in action (per docs union) — stringify if so
        if isinstance(action_val, tuple):
            action_val = '-'.join(map(str, action_val))

        normalized_changes.append({
            'change_class': getattr(ch, 'change_class', None),
            'lesson_n': lesson_display,
            'title': getattr(ch, 'title', None),
            'action': action_val
        })

    return render_template(
        'substitutions.html',
        changes=normalized_changes,
        student=student_data,
        student_class=class_name,  # provide for badge in template
        current_date=specific_date,
        prev_date=prev_date,
        next_date=next_date,
        CHANGE="change",  # keep legacy constants for template compatibility
        ADDITION="add",
        DELETION="remove"
    )

@app.route('/lunches/', defaults={'date_str': None})
@app.route('/lunches/<date_str>')
def lunches(date_str):
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    try:
        if date_str:
            specific_date = datetime.strptime(date_str, '%Y-%m-%d').date()
        else:
            specific_date = date.today()
    except ValueError:
        specific_date = date.today()

    prev_date = specific_date - timedelta(days=1)
    next_date = specific_date + timedelta(days=1)

    meals_data = edupage.get_meals(specific_date)

    students = edupage.get_students()
    if 'student_id' in session and session['student_id']:
        student_data = next((student for student in students if student.person_id == session['student_id']), None)
    else:
        student_data = next((student for student in students if student.name == session['username']), None)
    
    if meals_data is None:
        meals_list = []
    else:
        meals_list = []
        if meals_data.snack:
            meals_list.append(meals_data.snack)
        if meals_data.lunch:
            meals_list.append(meals_data.lunch)
        if meals_data.afternoon_snack:
            meals_list.append(meals_data.afternoon_snack)

    return render_template('lunches.html', 
                         meals=meals_list, 
                         current_date=specific_date,
                         student=student_data,
                         prev_date=prev_date,
                         next_date=next_date)

class Term(Enum):
    FIRST = "P1"
    SECOND = "P2"

@app.route('/grades')
def grades():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    year = edupage.get_school_year()
    grades_data = edupage.get_grades_for_term(year, Term.SECOND)
    students = edupage.get_students()
    if 'student_id' in session and session['student_id']:
        student_data = next((student for student in students if student.person_id == session['student_id']), None)
    else:
        student_data = next((student for student in students if student.name == session['username']), None)
    
    if grades_data:
        grades_data.sort(key=lambda x: x.date, reverse=True)

    return render_template('grades.html', grades=grades_data, student=student_data)

@app.route('/timetable/', defaults={'date_str': None})
@app.route('/timetable/<date_str>')
def get_timetable(date_str):
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    try:
        if date_str:
            specific_date = datetime.strptime(date_str, '%Y-%m-%d').date()
        else:
            specific_date = date.today()
    except ValueError:
        specific_date = date.today()

    prev_date = specific_date - timedelta(days=1)
    next_date = specific_date + timedelta(days=1)

    students = edupage.get_students()
    if 'student_id' in session and session['student_id']:
        student_data = next((student for student in students if student.person_id == session['student_id']), None)
    else:
        student_data = next((student for student in students if student.name == session['username']), None)
    
    if student_data is None:
        return redirect(url_for('index'))

    timetable = edupage.get_my_timetable(specific_date)

    if timetable is None:
        timetable = []

    # Calculate a 7-day range centered around current date
    date_range = []
    for i in range(-3, 4):
        date_range.append(specific_date + timedelta(days=i))

    return render_template('timetable.html', 
                           timetable=timetable, 
                           student=student_data, 
                           datetime=datetime,
                           timedelta=timedelta,
                           current_date=specific_date, 
                           prev_date=prev_date, 
                           next_date=next_date,
                           date_range=date_range)  # Add this line

EVENT_TYPE_MAP = {
    "album": "Album",
    "arrival_to_school": "Arrival to School",
    "bee": "Bee",
    "big_exam": "Big Exam",
    "booked_room": "Booked Room",
    "change_room": "Change Room",
    "classification_meeting": "Classification Meeting",
    "class_book": "Class Book",
    "class_teacher_event": "Class Teacher Event",
    "class_teacher_lesson": "Class Teacher Lesson",
    "confirmation": "Confirmation",
    "contest": "Contest",
    "culture": "Culture",
    "distant_learning": "Distant Learning",
    "event": "Event",
    "exam_assignment": "Exam Assignment",
    "exam_evaluation": "Exam Evaluation",
    "excursion": "Excursion",
    "excused_lesson": "Excused Lesson",
    "food_credit": "Food Credit",
    "strava_vydaj": "Food Served",
    "free_day": "Free Day",
    "grade": "Grade",
    "grades_doc": "Grades Document",
    "holiday": "Holiday",
    "homework": "Homework",
    "homework_student_state": "Homework Student State",
    "homework_test": "Homework Test",
    "h_attendance": "Attendance",
    "h_bee": "Bee",
    "h_clearcache": "Clear Cache",
    "h_cleardbi": "Clear DBI",
    "h_clearisicdata": "Clear ISIC Data",
    "h_contest": "Contest",
    "h_dailyplan": "Daily Plan",
    "h_edusettings": "Edu Settings",
    "h_financie": "Finances",
    "h_grades": "Grades",
    "h_homework": "Homework",
    "h_igroups": "Groups",
    "h_process": "Process",
    "h_processtypes": "Process Types",
    "h_settings": "Settings",
    "h_substitution": "Substitution",
    "h_timetable": "Timetable",
    "h_userphoto": "User Photo",
    "h_stravamenu": "Strava Menu",
    "h_dailyplan": "Daily Plan",
    "pipnutie": "Check In/Out",
    "h_clearplany": "Clear Plans",
    "sprava": "Message",
    "lesson": "Lesson",
    "message": "Message",
    "news": "News",
    "new_menu": "New Menu",
    "oral_exam": "Oral Exam",
    "other": "Other",
    "paper": "Paper",
    "parents_evening": "Parents' Evening",
    "poll": "Poll",
    "process": "Process",
    "project": "Project",
    "project_exam": "Project Exam",
    "project_lesson": "Project Lesson",
    "representation": "Representation",
    "safety_instructioning": "Safety Instructioning",
    "school_event": "School Event",
    "school_trip": "School Trip",
    "short_exam": "Short Exam",
    "short_holiday": "Short Holiday",
    "student_absent": "Student Absent",
    "substitution": "Substitution",
    "teacher_meeting": "Teacher Meeting",
    "testing": "Testing",
    "test_result": "Test Result",
    "testpridelenie": "Assigned Test",
    "timetable": "Timetable",
    "tt_cancel": "Timetable Cancel",
    "tutoring": "Tutoring",
    "znamka": "New Grade"
}

EVENT_TYPE_ICONS = {
    "album": "fa-photo-video",
    "arrival_to_school": "fa-school",
    "bee": "fa-bee",
    "big_exam": "fa-file-alt",
    "booked_room": "fa-door-closed",
    "change_room": "fa-exchange-alt",
    "classification_meeting": "fa-users",
    "class_book": "fa-book",
    "class_teacher_event": "fa-chalkboard-teacher",
    "class_teacher_lesson": "fa-chalkboard",
    "confirmation": "fa-check-circle",
    "contest": "fa-trophy",
    "culture": "fa-theater-masks",
    "distant_learning": "fa-laptop-house",
    "event": "fa-calendar-alt",
    "exam_assignment": "fa-tasks",
    "exam_evaluation": "fa-clipboard-check",
    "excursion": "fa-bus",
    "excused_lesson": "fa-user-check",
    "food_credit": "fa-utensils",
    "strava_vydaj": "fa-utensils",
    "free_day": "fa-sun",
    "grade": "fa-graduation-cap",
    "grades_doc": "fa-file-alt",
    "holiday": "fa-umbrella-beach",
    "homework": "fa-book-open",
    "homework_student_state": "fa-user-graduate",
    "homework_test": "fa-file-alt",
    "h_attendance": "fa-user-check",
    "h_bee": "fa-bee",
    "h_clearcache": "fa-trash-alt",
    "h_cleardbi": "fa-trash-alt",
    "h_clearisicdata": "fa-trash-alt",
    "h_contest": "fa-trophy",
    "h_dailyplan": "fa-calendar-day",
    "h_edusettings": "fa-cogs",
    "h_financie": "fa-money-bill-wave",
    "h_grades": "fa-graduation-cap",
    "h_homework": "fa-book-open",
    "h_igroups": "fa-users",
    "h_process": "fa-cogs",
    "h_processtypes": "fa-cogs",
    "h_settings": "fa-cogs",
    "h_substitution": "fa-exchange-alt",
    "h_timetable": "fa-calendar-alt",
    "h_userphoto": "fa-user",
    "h_stravamenu": "fa-utensils",
    "pipnutie": "fa-sign-in-alt",
    "h_clearplany": "fa-trash-alt",
    "sprava": "fa-envelope",
    "lesson": "fa-chalkboard",
    "message": "fa-envelope",
    "news": "fa-newspaper",
    "new_menu": "fa-bars",
    "oral_exam": "fa-microphone",
    "other": "fa-ellipsis-h",
    "paper": "fa-file-alt",
    "parents_evening": "fa-users",
    "poll": "fa-poll",
    "process": "fa-cogs",
    "project": "fa-project-diagram",
    "project_exam": "fa-file-alt",
    "project_lesson": "fa-chalkboard",
    "representation": "fa-users",
    "safety_instructioning": "fa-shield-alt",
    "school_event": "fa-school",
    "school_trip": "fa-bus",
    "short_exam": "fa-file-alt",
    "short_holiday": "fa-umbrella-beach",
    "student_absent": "fa-user-times",
    "substitution": "fa-exchange-alt",
    "teacher_meeting": "fa-users",
    "testing": "fa-vial",
    "test_result": "fa-clipboard-check",
    "testpridelenie": "fa-tasks",
    "timetable": "fa-calendar-alt",
    "tt_cancel": "fa-calendar-times",
    "tutoring": "fa-chalkboard-teacher",
    "znamka": "fa-graduation-cap"
}

if __name__ == '__main__':
    app.run(debug=True)