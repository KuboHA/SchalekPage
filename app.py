from flask import Flask, render_template, request, redirect, url_for, session, jsonify
from edupage_api import Edupage
from edupage_api.substitution import Substitution, TimetableChange
from edupage_api.grades import Grades, EduGrade
from edupage_api.lunches import Lunches  # Import the Lunches class
from datetime import datetime, date
from typing import Optional
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
        # Attempt to log in
        two_factor_login = edupage.login(username, password, subdomain)

        if two_factor_login:
            # Handle 2FA if needed
            session['subdomain'] = subdomain
            session['username'] = username
            session['password'] = password  # Store password in session
            session['session_id'] = edupage.session.cookies.get('PHPSESSID')
            return redirect(url_for('two_factor'))
        else:
            # Login successful, no 2FA needed
            session['subdomain'] = subdomain
            session['username'] = username
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
    student_data = (student for student in students if student.name == 'Jakub Schalek').__next__()
    
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

    return render_template('dashboard.html', student=student)

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
    specific_date = date(2025, 3, 3)
    changes = substitution.get_timetable_changes(specific_date)

    if changes is None:
        changes = []

    return render_template('timetable_changes.html', changes=changes)

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

    # Fetch grades for the logged-in student
    grades_module = Grades(edupage)
    grades_list = grades_module.get_grades(term=term, year=None)

    if grades_list is None:
        grades_list = []

    # Group grades by subject
    grades = defaultdict(list)
    for grade in grades_list:
        grades[grade.subject_name].append(grade)

    if request.headers.get('X-Requested-With') == 'XMLHttpRequest':
        return jsonify(grades=grades)

    return render_template('grades.html', grades=grades)

@app.route('/subjects', methods=['GET'])
def subjects():
     if 'subdomain' not in session or 'username' not in session or 'session_id' not in session:
        return redirect(url_for('index'))

    # Recreate the Edupage object
    edupage = Edupage()
    edupage.login(session['username'], session['password'], session['subdomain'])
    edupage.session.cookies.set('PHPSESSID', session['session_id'])

    subjects = Subject(edupage)
    

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

if __name__ == '__main__':
    app.run(debug=True)


