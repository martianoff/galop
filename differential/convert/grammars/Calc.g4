// A calculator: labeled alternatives with rule and token labels, list labels
// on rules and on token sets, optional and nested blocks, and a dangling else.
grammar Calc;

prog : stat+ EOF ;
stat : target=ID '=' value=expr ';'                       # Assign
     | 'print' args+=expr (',' args+=expr)* ';'           # Print
     | '{' body+=stat* '}'                                # Block
     | 'if' cond=expr then=stat ('else' otherwise=stat)?  # If
     ;
expr : left=term (ops+=('+' | '-') rights+=term)* ;
term : factor (op=('*' | '/') factor)* ;
factor : '-' inner=factor | atom=primary ;
primary : INT
        | name=ID '(' (args+=expr (',' args+=expr)*)? ')'
        | ID
        | '(' expr ')'
        ;

ID : [a-z]+ ;
INT : [0-9]+ ;
WS : [ \t\r\n]+ -> skip ;
