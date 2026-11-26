package nanoandroid.compat;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

@Retention(RetentionPolicy.RUNTIME)
@Target({ElementType.TYPE, ElementType.METHOD})
public @interface NanoApiStatus {
    ApiSupportLevel level();
    int sinceApi() default 26;
    int untilApi() default 37;
    String fallback() default "";
    String notes() default "";
}
