// EOF in lexer rules (#93): a line comment or a block may end at the end of
// the input instead of at a newline or a closing brace.
grammar Lines;

prog : stmt* EOF ;
stmt : ID '=' value ';' | LINE | BLOCK ;
value : ID | NUM | '(' value ')' ;

LINE : '//' ~[\n]* ('\n' | EOF) ;
BLOCK : '{' BODY ('}' | EOF) ;
fragment BODY : ( '{' BODY '}' | ~[{}] )* ;
ID : [a-z]+ ;
NUM : [0-9]+ ;
WS : [ \n]+ -> skip ;
