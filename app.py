from flask import Flask, render_template, request, redirect, url_for, session, jsonify
from edupage_api import Edupage
from edupage_api.people import EduAccount  # Import the EduAccount class
from edupage_api.messages import Messages  # Import the Messages class
from edupage_api.substitution import Substitution, TimetableChange
from edupage_api.grades import Grades, EduGrade
from edupage_api.lunches import Lunches  # Import the Lunches class
from datetime import datetime, date
from typing import Optional, Union
from collections import defaultdict

app = Flask(__name__)
app.secret_key = '738-531-827'  # Change this to a real secret key in production

class EduStudent:
    def __init__(self, person_id: int, name: str, gender: str, in_school_since: Optional[datetime], class_id: int, number_in_class: int):
        self.person_id = person_id
        self.name = name
        self.gender = gender
        self.in_school_since = in_school_since
        self.class_id = class_id
        self.number_in_class = number_in_class

def fetch_teacher_data_by_name(edupage: Edupage, teacher_name: str) -> Optional[dict]:
    # Assuming there is a method called `get_teachers` that returns a list of teachers
    teachers = edupage.get_teachers()

    for teacher in teachers:
        if edupage.get_full_name(teacher) in teacher_name:
            teacher_data = teacher
            teacher_data["id"] = teacher.get('id')  # Assuming each teacher has an 'id' attribute
            return teacher_data
    return None

@app.route('/')
def index():
    return render_template('login.html')

@app.route('/login', methods=['POST'])
def login():
    username = request.form['username']
    password = request.form['password']
    subdomain = request.form['subdomain']

    # Format the username from NameSurname to Name Surname
    formatted_username = ' '.join([username[:username.find('S')], username[username.find('S'):]])

    edupage = Edupage()
    try:
        # Attempt to log in
        two_factor_login = edupage.login(username, password, subdomain)

        if two_factor_login:
            # Handle 2FA if needed
            session['subdomain'] = subdomain
            session['username'] = formatted_username
            session['password'] = password  # Store password in session
            session['session_id'] = edupage.session.cookies.get('PHPSESSID')
            return redirect(url_for('two_factor'))
        else:
            # Login successful, no 2FA needed
            session['subdomain'] = subdomain
            session['username'] = formatted_username
            session['password'] = password  # Store password in session
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
            # Recreate the Edupage object
            edupage.login(session['username'], session['password'], session['subdomain'])
            edupage.session.cookies.set('PHPSESSID', session['session_id'])

            # Finish 2FA
            two_factor_login = edupage.login(session['username'], session['password'], session['subdomain'])
            two_factor_login.finish_with_code(code)

            # Update session data
            session['session_id'] = edupage.session.cookies.get('PHPSESSID')
            return redirect(url_for('dashboard'))
        except Exception as e:
            return render_template('two_factor.html', error=str(e))

    return render_template('two_factor.html')

@app.route('/dashboard')
def dashboard():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    # Recreate the Edupage object
    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    # Fetch student data from Edupage API
    students = edupage.get_students()  # Replace this with the actual method to fetch student data
    student_data = next((student for student in students if student.name == session['username']), None)
    
    if student_data is None:
        return redirect(url_for('index'))

    student = EduStudent(
        person_id=student_data.person_id,
        name=student_data.name,
        gender=student_data.gender,
        in_school_since=student_data.in_school_since,
        class_id=student_data.class_id,
        number_in_class=student_data.number_in_class
    )

    # Fetch notifications
    notifications = edupage.get_notifications()

    return render_template('dashboard.html', student=student, notifications=notifications, event_type_map=EVENT_TYPE_MAP, event_type_icons=EVENT_TYPE_ICONS)

@app.route('/search_teacher')
def search_teacher():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    teacher_name = request.args.get('name')
    if teacher_name:
        # Recreate the Edupage object
        edupage = Edupage()
        edupage.login(session['username'], session['password'], session['subdomain'])
        edupage.session.cookies.set('PHPSESSID', session['session_id'])

        teacher_data = fetch_teacher_data_by_name(edupage, teacher_name)
        if teacher_data:
            return f"Teacher found: {teacher_data['first_name']} {teacher_data['last_name']}"
        else:
            return "Teacher not found"
    return redirect(url_for('dashboard'))

@app.route('/timetable_changes', methods=['GET'])
def timetable_changes():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    # Recreate the Edupage object
    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    # Fetch timetable changes for a specific date
    substitution = Substitution(edupage)
    specific_date = date.today()
    changes = substitution.get_timetable_changes(specific_date)

    if changes is None:
        changes = []

    changes = [change for change in changes if change.change_class == '2.A']

    today_date = specific_date.strftime("%A, %B %d, %Y")

    return render_template('timetable_changes.html', changes=changes, today_date=today_date)

@app.route('/grades', methods=['GET'])
def grades():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    # Recreate the Edupage object
    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    # Get the term from the query parameter
    term = request.args.get('term', default=2, type=int)

    # Fetch grades for the logged-in student for the specified term
    grades_list = edupage.get_grades_for_term(year=None, term=term)

    if grades_list is None:
        grades_list = []

    # Group grades by subject
    grades = defaultdict(list)
    for grade in grades_list:
        grades[grade.subject_name].append(grade)

    if request.headers.get('X-Requested-With') == 'XMLHttpRequest':
        return jsonify(grades=grades)

    return render_template('grades.html', grades=grades)

@app.route('/lunches', methods=['GET'])
def lunches():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    lunches_module = Lunches(edupage)
    specific_date = date(2025, 3, 3)
    lunches = lunches_module.get_meals(specific_date)

    # Extract the meals list properly
    if hasattr(lunches, 'meals'):
        lunches = lunches.meals  # Extract meals if it's a structured object
    else:
        lunches = []

    return render_template('lunches.html', lunches=lunches)

@app.route('/timetable', methods=['GET'])
def get_timetable():
    if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    # Recreate the Edupage object
    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    # Fetch timetable for the logged-in student
    specific_date = date.today()
    teachers = edupage.get_teachers()
    students = edupage.get_students()
    if not students:
        return redirect(url_for('index'))
    student_data = next((student for student in students if student.name == session['username']), None)
    if student_data is None:
        return redirect(url_for('index'))
    EduStudent = student_data
    timetable = edupage.get_timetable(EduStudent, specific_date)

    if timetable is None:
        timetable = []
    

    return render_template('timetable.html', timetable=timetable, current_date=specific_date, teachers=teachers)


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

