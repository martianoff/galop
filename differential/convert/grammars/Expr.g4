// A small expression grammar: labeled alternatives, a list loop and a token
// label on a set inside a loop.
grammar Expr;

prog : stat+ EOF ;
stat : expr ';' # exprStat
     | ID '=' expr ';' # assign
     ;
expr : term (op=('+' | '-') term)* ;
term : ID | INT | '(' expr ')' ;

ID : [a-z]+ ;
INT : [0-9]+ ;
WS : [ \t\r\n]+ -> skip ;
