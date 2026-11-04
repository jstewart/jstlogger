// Package indexfmt parses mutt-style QSO index formats, which choose the
// columns of the QSO index, and turns a QSO into the cells and header
// titles of those columns. Laying the cells out (padding, truncation and
// sizing the flexible column) is left to the caller.
//
// A format is literal text mixed with expandos, each a '%' followed by an
// optional '-' (left-align; values are right-aligned otherwise), an
// optional width, and a case-sensitive letter or a braced time layout:
//
//	%X            expando X at its natural width
//	%NX           expando X in a column N cells wide (N > 0)
//	%-NX          the same, left-aligned
//	%*X           expando X in the column that takes the remaining width;
//	              at most one per format
//	%{LAYOUT}     the QSO start time in UTC, formatted with LAYOUT
//	%%            a literal '%'
//
// The expandos and their header titles are:
//
//	%Z    mark: "*" when marked, else " "; 1 cell wide by default  (none)
//	%c    call                                                     Call
//	%m    submode, or mode when there is no submode                Mode
//	%s    RST sent                                                 RST S
//	%r    RST received                                             RST R
//	%b    band                                                     Band
//	%f    frequency in MHz, e.g. 14.074                            Freq
//	%C    country                                                  Country
//	%S    station callsign                                         Station
//	%o    operator                                                 Op
//	%n    name                                                     Name
//	%g    grid square                                              Grid
//	%D    distance, rounded, in the configured unit                Dist
//	%k    continent                                                Cont
//	%x    DXCC entity number                                       DXCC
//	%{}   start time                                               Time or Date
//
// A %{} column is titled Time when its layout has only time-of-day
// conversions (%H, %M, %S) and Date otherwise. Layouts use this strftime
// subset:
//
//	%Y  year, e.g. 2024             %H  hour, 00-23
//	%y  year in the century, 00-99  %M  minute, 00-59
//	%m  month, 01-12                %S  second, 00-59
//	%d  day of the month, 01-31     %b  month name, e.g. Mar
//	%e  day of the month, " 1"-31   %B  month name, e.g. March
//	%j  day of the year, 001-366    %a  weekday name, e.g. Tue
//	%%  a literal '%'               %A  weekday name, e.g. Tuesday
//
// Default reproduces the built-in index:
//
//	%Z %-10{%Y-%m-%d} %-5{%H:%M} %-10c %-6m %-5s %-5r %-5b %-*C %-10S
//
// Parse reports malformed formats (an unknown expando or conversion, a
// lone '%', an unterminated '{', a zero width or a second flexible column)
// with errors that wrap ErrSyntax and name the 1-based column of the
// offending character.
package indexfmt
