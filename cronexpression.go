/*!
 * Copyright 2013 Raymond Hill
 *
 * Project: github.com/gorhill/cronexpression
 * File: cronexpression.go
 * Version: 1.0
 * License: GPL v3 see <https://www.gnu.org/licenses/gpl.html>
 *
 */

package cronexpression

/******************************************************************************/

import (
    "regexp"
    "sort"
    "strconv"
    "strings"
    "time"
)

/******************************************************************************/

// A CronExpression represents a specific cron expression as defined on
// Wikipedia: https://en.wikipedia.org/wiki/Cron#CRON_expression
//
type CronExpression struct {
    Expression             string
    secondList             []int
    minuteList             []int
    hourList               []int
    daysOfMonth            map[int]bool
    workdaysOfMonth        map[int]bool
    lastDayOfMonth         bool
    daysOfMonthRestricted  bool
    actualDaysOfMonthList  []int
    monthList              []int
    daysOfWeek             map[int]bool
    specificWeekDaysOfWeek map[int]bool
    lastWeekDaysOfWeek     map[int]bool
    daysOfWeekRestricted   bool
    yearList               []int
}

/******************************************************************************/

var noMatchTime = time.Date(2100, 7, 1, 0, 0, 0, 0, time.UTC)

/******************************************************************************/

// NewCronExpression() returns a new CronExpression pointer. It expects
// a well-formed cron expression. If a malformed cron expression is
// supplied, the result is undefined.
func NewCronExpression(cronLine string) *CronExpression {
    cronLineNormalized := cronNormalize(cronLine)

    // Split into fields
    cronFields := regexp.MustCompile(`\s+`).Split(cronLineNormalized, -1)

    // Our cron expression parser expects 7 fields:
    //    second minute hour dayofmonth month dayofweek year
    // Standard cron is 6 fields with year field being optional
    //           minute hour dayofmonth month dayofweek {year}
    // Thus...
    // If we have 5 fields, append wildcard year field
    if len(cronFields) < 6 {
        cronFields = append(cronFields, "*")
    }
    // If we have 6 fields, prepend match-once second field
    if len(cronFields) < 7 {
        cronFields = append(cronFields, "")
        copy(cronFields[1:], cronFields[0:])
        cronFields[0] = "0"
    }
    // We should have 7 fields at this point
    if len(cronFields) != 7 {
        panic("Malformed cron expression\n")
    }

    // Generic parser can be used for most fields
    cronExpr := &CronExpression{
        Expression: cronLine,
        secondList: genericFieldParse(cronFields[0], 0, 59),
        minuteList: genericFieldParse(cronFields[1], 0, 59),
        hourList:   genericFieldParse(cronFields[2], 0, 23),
        monthList:  genericFieldParse(cronFields[4], 1, 12),
        yearList:   genericFieldParse(cronFields[6], 1970, 2099),
    }

    // Days of month/days of week is a bit more complicated, due
    // to their extended syntax, and the fact that days per
    // month is a variable quantity, and relation between
    // days of week and days of month depends on the month/year.
    cronExpr.dayofmonthFieldParse(cronFields[3])
    cronExpr.dayofweekFieldParse(cronFields[5])

    return cronExpr
}

/******************************************************************************/

// NextTimeFromCronString() returns the time stamp following fromTime which
// satisfies the cron expression cronLine. If no matching time stamp is found,
// using NoMatch() with the returned time stamp as argument will return true.
//
// If the same cron expression must be used repeatedly, it is better to use
// NewCronExpression() in order to avoid overhead of cron expression parsing.
func NextTimeFromCronString(cronLine string, fromTime time.Time) time.Time {
    cronexpr := NewCronExpression(cronLine)
    return cronexpr.NextTime(fromTime)
}

/******************************************************************************/

// NextTimeNFromCronString() returns the n time stamps following fromTime which
// satisfy the cron expression cronLine. An empty list is returned if
// there is no matching time stamp.
//
// If the same cron expression must be used repeatedly, it is better to use
// NewCronExpression() in order to avoid overhead of cron expression parsing.
func NextTimeNFromCronString(cronLine string, fromTime time.Time, n int) []time.Time {
    cronexpr := NewCronExpression(cronLine)
    return cronexpr.NextTimeN(fromTime, n)
}

/******************************************************************************/

// CronExpression.NextTime() returns the time stamp following fromTime which
// satisfies the cron expression. If no matching time stamp is found,
// using NoMatch() with the returned time stamp as argument will return true.
func (cronexpr *CronExpression) NextTime(fromTime time.Time) time.Time {
    // Special case
    if NoMatch(fromTime) {
        return fromTime
    }

    // First we need to ensure supplied time stamp matches
    // the cron expression. If not, this means the supplied time
    // stamp might be between matching time stamps, thus we move
    // to closest matching time stamp without changing time stamp

    // year
    v := fromTime.Year()
    i := sort.SearchInts(cronexpr.yearList, v)
    if i == len(cronexpr.yearList) {
        return noMatchTime
    }
    if v != cronexpr.yearList[i] {
        return cronexpr.nextYear(fromTime)
    }
    // month
    v = int(fromTime.Month())
    i = sort.SearchInts(cronexpr.monthList, v)
    if i == len(cronexpr.monthList) {
        return cronexpr.nextYear(fromTime)
    }
    if v != cronexpr.monthList[i] {
        return cronexpr.nextMonth(fromTime)
    }

    cronexpr.actualDaysOfMonthList = cronexpr.calculateActualDaysOfMonth(fromTime.Year(), int(fromTime.Month()))
    if len(cronexpr.actualDaysOfMonthList) == 0 {
        return cronexpr.nextMonth(fromTime)
    }

    // day of month
    v = fromTime.Day()
    i = sort.SearchInts(cronexpr.actualDaysOfMonthList, v)
    if i == len(cronexpr.actualDaysOfMonthList) {
        return cronexpr.nextMonth(fromTime)
    }
    if v != cronexpr.actualDaysOfMonthList[i] {
        return cronexpr.nextDayOfMonth(fromTime)
    }
    // hour
    v = fromTime.Hour()
    i = sort.SearchInts(cronexpr.hourList, v)
    if i == len(cronexpr.hourList) {
        return cronexpr.nextDayOfMonth(fromTime)
    }
    if v != cronexpr.hourList[i] {
        return cronexpr.nextHour(fromTime)
    }
    // minute
    v = fromTime.Minute()
    i = sort.SearchInts(cronexpr.minuteList, v)
    if i == len(cronexpr.minuteList) {
        return cronexpr.nextHour(fromTime)
    }
    if v != cronexpr.minuteList[i] {
        return cronexpr.nextMinute(fromTime)
    }
    // second
    v = fromTime.Second()
    i = sort.SearchInts(cronexpr.secondList, v)
    if i == len(cronexpr.secondList) {
        return cronexpr.nextMinute(fromTime)
    }

    // If we reach this point, there is nothing better to do
    // than to move to the next second

    return cronexpr.nextSecond(fromTime)
}

/******************************************************************************/

// CronExpression.NextTimeN() returns an array of n time stamps following
// fromTime which satisfy the cron expression. An empty list is returned if
// there is no matching time stamp.
func (cronexpr *CronExpression) NextTimeN(fromTime time.Time, n int) []time.Time {
    if n <= 0 {
        panic("CronExpression.NextTimeN(): invalid count")
    }
    nextTimes := make([]time.Time, 0)
    fromTime = cronexpr.NextTime(fromTime)
    for {
        if NoMatch(fromTime) {
            break
        }
        nextTimes = append(nextTimes, fromTime)
        n -= 1
        if n == 0 {
            break
        }
        fromTime = cronexpr.nextSecond(fromTime)
    }
    return nextTimes
}

/******************************************************************************/

// NoMatch() returns whether t is a valid time stamp, from CronExpression point
// of view.
func NoMatch(t time.Time) bool {
    // https://en.wikipedia.org/wiki/Cron#CRON_expression: 1970–2099
    return t.Year() >= 2100
}

/******************************************************************************/

func (cronexpr *CronExpression) nextYear(t time.Time) time.Time {
    // Find index at which item in list is greater or equal to
    // candidate year
    i := sort.SearchInts(cronexpr.yearList, t.Year()+1)
    if i == len(cronexpr.yearList) {
        return noMatchTime
    }
    // Year changed, need to recalculate actual days of month
    cronexpr.actualDaysOfMonthList = cronexpr.calculateActualDaysOfMonth(cronexpr.yearList[i], cronexpr.monthList[0])
    if len(cronexpr.actualDaysOfMonthList) == 0 {
        return cronexpr.nextMonth(time.Date(
            cronexpr.yearList[i],
            time.Month(cronexpr.monthList[0]),
            1,
            cronexpr.hourList[0],
            cronexpr.minuteList[0],
            cronexpr.secondList[0],
            0,
            time.Local))
    }
    return time.Date(
        cronexpr.yearList[i],
        time.Month(cronexpr.monthList[0]),
        cronexpr.actualDaysOfMonthList[0],
        cronexpr.hourList[0],
        cronexpr.minuteList[0],
        cronexpr.secondList[0],
        0,
        time.Local)
}

/******************************************************************************/

func (cronexpr *CronExpression) nextMonth(t time.Time) time.Time {
    // Find index at which item in list is greater or equal to
    // candidate month
    i := sort.SearchInts(cronexpr.monthList, int(t.Month())+1)
    if i == len(cronexpr.monthList) {
        return cronexpr.nextYear(t)
    }
    // Month changed, need to recalculate actual days of month
    cronexpr.actualDaysOfMonthList = cronexpr.calculateActualDaysOfMonth(t.Year(), cronexpr.monthList[i])
    if len(cronexpr.actualDaysOfMonthList) == 0 {
        return cronexpr.nextMonth(time.Date(
            t.Year(),
            time.Month(cronexpr.monthList[i]),
            1,
            cronexpr.hourList[0],
            cronexpr.minuteList[0],
            cronexpr.secondList[0],
            0,
            time.Local))
    }

    return time.Date(
        t.Year(),
        time.Month(cronexpr.monthList[i]),
        cronexpr.actualDaysOfMonthList[0],
        cronexpr.hourList[0],
        cronexpr.minuteList[0],
        cronexpr.secondList[0],
        0,
        time.Local)
}

/******************************************************************************/

func (cronexpr *CronExpression) calculateActualDaysOfMonth(year, month int) []int {
    actualDaysOfMonthMap := make(map[int]bool)
    timeOrigin := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
    lastDayOfMonth := timeOrigin.AddDate(0, 1, -1).Day()

    // As per crontab man page (http://linux.die.net/man/5/crontab#):
    //  "The day of a command's execution can be specified by two
    //  "fields - day of month, and day of week. If both fields are
    //  "restricted (ie, aren't *), the command will be run when
    //  "either field matches the current time"
    if cronexpr.daysOfMonthRestricted || cronexpr.daysOfWeekRestricted == false {
        // Last day of month
        if cronexpr.lastDayOfMonth {
            actualDaysOfMonthMap[lastDayOfMonth] = true
        }
        // Days of month
        for v, _ := range cronexpr.daysOfMonth {
            // Ignore days beyond end of month
            if v <= lastDayOfMonth {
                actualDaysOfMonthMap[v] = true
            }
        }
        // Work days of month
        // As per Wikipedia: month boundaries are not crossed.
        for v, _ := range cronexpr.workdaysOfMonth {
            // Ignore days beyond end of month
            if v <= lastDayOfMonth {
                // If saturday, then friday
                if timeOrigin.AddDate(0, 0, v-1).Weekday() == time.Saturday {
                    if v > 1 {
                        v -= 1
                    } else {
                        v += 2
                    }
                    // If sunday, then monday
                } else if timeOrigin.AddDate(0, 0, v-1).Weekday() == time.Sunday {
                    if v < lastDayOfMonth {
                        v += 1
                    } else {
                        v -= 2
                    }
                }
                actualDaysOfMonthMap[v] = true
            }
        }
    }

    if cronexpr.daysOfWeekRestricted {
        // How far first sunday is from first day of month
        offset := 7 - int(timeOrigin.Weekday())
        // days of week
        //  offset : (7 - day_of_week_of_1st_day_of_month)
        //  target : (7 * week_of_month) + (offset + day_of_week) % 7 + 1
        for w := 0; w <= 4; w += 1 {
            for v, _ := range cronexpr.daysOfWeek {
                v := 1 + w*7 + (offset+v)%7
                if v <= lastDayOfMonth {
                    actualDaysOfMonthMap[v] = true
                }
            }
        }
        // days of week of specific week in the month
        //  offset : (7 - day_of_week_of_1st_day_of_month)
        //  target : (7 * week_of_month) + (offset + day_of_week) % 7 + 1
        for v, _ := range cronexpr.specificWeekDaysOfWeek {
            v := 1 + 7*(v/7) + (offset+v)%7
            if v <= lastDayOfMonth {
                actualDaysOfMonthMap[v] = true
            }
        }
        // Last days of week of the month
        lastWeekOrigin := timeOrigin.AddDate(0, 1, -7)
        offset = 7 - int(lastWeekOrigin.Weekday())
        for v, _ := range cronexpr.lastWeekDaysOfWeek {
            v := lastWeekOrigin.Day() + (offset+v)%7
            if v <= lastDayOfMonth {
                actualDaysOfMonthMap[v] = true
            }
        }
    }

    return toList(actualDaysOfMonthMap)
}

/******************************************************************************/

func (cronexpr *CronExpression) nextDayOfMonth(t time.Time) time.Time {
    // Find index at which item in list is greater or equal to
    // candidate day of month
    i := sort.SearchInts(cronexpr.actualDaysOfMonthList, t.Day()+1)
    if i == len(cronexpr.actualDaysOfMonthList) {
        return cronexpr.nextMonth(t)
    }

    return time.Date(
        t.Year(),
        t.Month(),
        cronexpr.actualDaysOfMonthList[i],
        cronexpr.hourList[0],
        cronexpr.minuteList[0],
        cronexpr.secondList[0],
        0,
        t.Location())
}

/******************************************************************************/

func (cronexpr *CronExpression) nextHour(t time.Time) time.Time {
    // Find index at which item in list is greater or equal to
    // candidate hour
    i := sort.SearchInts(cronexpr.hourList, t.Hour()+1)
    if i == len(cronexpr.hourList) {
        return cronexpr.nextDayOfMonth(t)
    }

    return time.Date(
        t.Year(),
        t.Month(),
        t.Day(),
        cronexpr.hourList[i],
        cronexpr.minuteList[0],
        cronexpr.secondList[0],
        0,
        t.Location())
}

/******************************************************************************/

func (cronexpr *CronExpression) nextMinute(t time.Time) time.Time {
    // Find index at which item in list is greater or equal to
    // candidate minute
    i := sort.SearchInts(cronexpr.minuteList, t.Minute()+1)
    if i == len(cronexpr.minuteList) {
        return cronexpr.nextHour(t)
    }

    return time.Date(
        t.Year(),
        t.Month(),
        t.Day(),
        t.Hour(),
        cronexpr.minuteList[i],
        cronexpr.secondList[0],
        0,
        t.Location())
}

/******************************************************************************/

func (cronexpr *CronExpression) nextSecond(t time.Time) time.Time {
    // nextSecond() assumes all other fields are exactly matched
    // to the cron expression

    // Find index at which item in list is greater or equal to
    // candidate second
    i := sort.SearchInts(cronexpr.secondList, t.Second()+1)
    if i == len(cronexpr.secondList) {
        return cronexpr.nextMinute(t)
    }

    return time.Date(
        t.Year(),
        t.Month(),
        t.Day(),
        t.Hour(),
        t.Minute(),
        cronexpr.secondList[i],
        0,
        t.Location())
}

/******************************************************************************/

var cronNormalizer = strings.NewReplacer(
    // Order is important!
    "@yearly", "0 0 0 1 1 * *",
    "@annually", "0 0 0 1 1 * *",
    "@monthly", "0 0 0 1 * * *",
    "@weekly", "0 0 0 * * 0 *",
    "@daily", "0 0 0 * * * *",
    "@hourly", "0 0 * * * * *",
    "january", "1",
    "february", "2",
    "march", "3",
    "april", "4",
    "may", "5",
    "june", "6",
    "july", "7",
    "august", "8",
    "september", "9",
    "october", "0",
    "november", "1",
    "december", "2",
    "sunday", "0",
    "monday", "1",
    "tuesday", "2",
    "wednesday", "3",
    "thursday", "4",
    "friday", "5",
    "saturday", "6",
    "jan", "1",
    "feb", "2",
    "mar", "3",
    "apr", "4",
    "jun", "6",
    "jul", "7",
    "aug", "8",
    "sep", "9",
    "oct", "0",
    "nov", "1",
    "dec", "2",
    "sun", "0",
    "mon", "1",
    "tue", "2",
    "wed", "3",
    "thu", "4",
    "fri", "5",
    "sat", "6",
    "?", "*")

func cronNormalize(cronLine string) string {
    cronLine = strings.TrimSpace(cronLine)
    cronLine = strings.ToLower(cronLine)
    cronLine = cronNormalizer.Replace(cronLine)
    return cronLine
}

/******************************************************************************/

func (cronexpr *CronExpression) dayofweekFieldParse(cronField string) error {
    // Defaults
    cronexpr.daysOfWeekRestricted = true
    cronexpr.lastWeekDaysOfWeek = make(map[int]bool)
    cronexpr.daysOfWeek = make(map[int]bool)

    // "You can also mix all of the above, as in: 1-5,10,12,20-30/5"
    cronList := strings.Split(cronField, ",")
    for _, s := range cronList {
        // "/"
        step, s := extractInterval(s)
        // "*"
        if s == "*" {
            cronexpr.daysOfWeekRestricted = (step > 1)
            populateMany(cronexpr.daysOfWeek, 0, 6, step)
            continue
        }
        // "-"
        // week day interval for all weeks
        i := strings.Index(s, "-")
        if i >= 0 {
            min := atoi(s[:i]) % 7
            max := atoi(s[i+1:]) % 7
            populateMany(cronexpr.daysOfWeek, min, max, step)
            continue
        }
        // single value
        // "l": week day for last week
        i = strings.Index(s, "l")
        if i >= 0 {
            populateOne(cronexpr.lastWeekDaysOfWeek, atoi(s[:i])%7)
            continue
        }
        // "#": week day for specific week
        i = strings.Index(s, "#")
        if i >= 0 {
            // v#w
            v := atoi(s[:i]) % 7
            w := atoi(s[i+1:])
            // v domain = [0,7]
            // w domain = [1,5]
            populateOne(cronexpr.specificWeekDaysOfWeek, (w-1)*7+(v%7))
            continue
        }
        // week day interval for all weeks
        if step > 0 {
            v := atoi(s) % 7
            populateMany(cronexpr.daysOfWeek, v, 6, step)
            continue
        }
        // single week day for all weeks
        v := atoi(s) % 7
        populateOne(cronexpr.daysOfWeek, v)
    }

    return nil
}

/******************************************************************************/

func (cronexpr *CronExpression) dayofmonthFieldParse(cronField string) error {
    // Defaults
    cronexpr.daysOfMonthRestricted = true
    cronexpr.lastDayOfMonth = false

    cronexpr.daysOfMonth = make(map[int]bool)     // days of month map
    cronexpr.workdaysOfMonth = make(map[int]bool) // work day of month map

    // "You can also mix all of the above, as in: 1-5,10,12,20-30/5"
    cronList := strings.Split(cronField, ",")
    for _, s := range cronList {
        // "/"
        step, s := extractInterval(s)
        // "*"
        if s == "*" {
            cronexpr.daysOfMonthRestricted = (step > 1)
            populateMany(cronexpr.daysOfMonth, 1, 31, step)
            continue
        }
        // "-"
        i := strings.Index(s, "-")
        if i >= 0 {
            populateMany(cronexpr.daysOfMonth, atoi(s[:i]), atoi(s[i+1:]), step)
            continue
        }
        // single value
        // "l": last day of month
        if s == "l" {
            cronexpr.lastDayOfMonth = true
            continue
        }
        // "w": week day
        i = strings.Index(s, "w")
        if i >= 0 {
            populateOne(cronexpr.workdaysOfMonth, atoi(s[:i]))
            continue
        }
        // single value with interval
        if step > 0 {
            populateMany(cronexpr.daysOfMonth, atoi(s), 31, step)
            continue
        }
        // single value
        populateOne(cronexpr.daysOfMonth, atoi(s))
    }

    return nil
}

/******************************************************************************/

func genericFieldParse(cronField string, min, max int) []int {
    // Defaults
    values := make(map[int]bool)

    // "You can also mix all of the above, as in: 1-5,10,12,20-30/5"
    cronList := strings.Split(cronField, ",")
    for _, s := range cronList {
        // "/"
        step, s := extractInterval(s)
        // "*"
        if s == "*" {
            populateMany(values, min, max, step)
            continue
        }
        // "-"
        i := strings.Index(s, "-")
        if i >= 0 {
            populateMany(values, atoi(s[:i]), atoi(s[i+1:]), step)
            continue
        }
        // single value with interval
        if step > 0 {
            populateMany(values, atoi(s), max, step)
            continue
        }
        // single value
        populateOne(values, atoi(s))
    }

    return toList(values)
}

/******************************************************************************/

// Local helpers

func extractInterval(s string) (int, string) {
    step := 0
    i := strings.Index(s, "/")
    if i >= 0 {
        step = atoi(s[i+1:])
        s = s[:i]
    }
    return step, s
}

func atoi(s string) int {
    v, err := strconv.Atoi(s)
    if err != nil {
        panic(err)
    }
    return v
}

func populateOne(values map[int]bool, v int) {
    values[v] = true
}

func populateMany(values map[int]bool, min, max, step int) {
    if step == 0 {
        step = 1
    }
    for i := min; i <= max; i += step {
        values[i] = true
    }
}

func toList(set map[int]bool) []int {
    list := make([]int, len(set))
    i := 0
    for k, _ := range set {
        list[i] = k
        i += 1
    }
    sort.Ints(list)
    return list
}